package chiandns

import (
	goc "github.com/Chenyao2333/golang-cache"
	"github.com/alekseiapa/China-Dns/loggerconfig"
	"github.com/miekg/dns"
	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

type ServerConfig struct {
	PrimaryDNS   string
	SecondaryDNS string
	ListenAddr   string
}

type DNServer struct {
	config           ServerConfig
	dnsServers       [2]*dns.Server
	chinaDomainCache *goc.Cache
	responseCache    *goc.Cache
}

type DNServerError string

var geoIPDB *geoip2.Reader
var logger = loggerconfig.NewLogger()

const minimumTTL = 60

func init() {
	var err error
	geoIPDB, err = geoip2.Open("GeoLite2-Country.mmdb")
	if err != nil {
		logger.Fatalln(err)
	}
}

func (e DNServerError) Error() string {
	return string(e)
}

func NewDNServer(cfg ServerConfig) (*DNServer, error) {
	server := &DNServer{}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:53"
	}
	server.config = cfg

	server.dnsServers[0] = &dns.Server{
		Addr: cfg.ListenAddr,
		Net:  "udp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			server.handleDNSRequest(w, req, "udp")
		}),
	}

	server.dnsServers[1] = &dns.Server{
		Addr: cfg.ListenAddr,
		Net:  "tcp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			server.handleDNSRequest(w, req, "tcp")
		}),
	}

	var err error
	server.chinaDomainCache, err = goc.NewCache("lru", 1024*20)
	if err != nil {
		logger.Fatalln(err)
	}

	server.responseCache, err = goc.NewCache("lru", 1024*20)
	if err != nil {
		logger.Fatalln(err)
	}

	return server, nil
}

func (server *DNServer) Start() error {
	errorChannel := make(chan error, 2)

	for _, dnsServer := range server.dnsServers {
		go func(ds *dns.Server) {
			errorChannel <- ds.ListenAndServe()
		}(dnsServer)
	}

	select {
	case err := <-errorChannel:
		return err
	}
}

func (server *DNServer) handleDNSRequest(writer dns.ResponseWriter, request *dns.Msg, network string) {
	response := &dns.Msg{}
	var err error
	var resolved bool

	if len(request.Question) < 1 {
		response.SetRcode(request, dns.RcodeNameError)
		writer.WriteMsg(response)
		return
	}

	qname := request.Question[0].Name
	var upstreamSource string

	// Check local hosts file, cache, or resolve via network
	if response, resolved = server.LookupHosts(request); resolved {
		upstreamSource = "hosts"
	} else if response, resolved = server.LookupCache(request, network); resolved {
		upstreamSource = "cache"
	} else {
		response, upstreamSource, err = server.LookupNetwork(request, network)
		if err != nil {
			logger.Error(err)
		}
		if response == nil {
			response = createServerFailureResponse(request)
		}
	}

	response.SetRcode(request, response.Rcode)

	logEntry := logger.WithFields(logrus.Fields{
		"action":     "handleDNSRequest",
		"domain":     qname,
		"queryType":  dns.TypeToString[request.Question[0].Qtype],
		"upstream":   upstreamSource,
		"statusCode": dns.RcodeToString[response.Rcode],
	})
	if response.Rcode == dns.RcodeSuccess {
		logEntry.Info()
	} else {
		logEntry.Warn()
	}

	writer.WriteMsg(response)
}

// LookupNetwork resolves the DNS request through the network.
// The function returns the DNS response, the name of the used upstream server, and an error, if any.
// In the current implementation, the error is always nil.
func (server *DNServer) LookupNetwork(request *dns.Msg, network string) (*dns.Msg, string, error) {
	primaryDNSChan := make(chan *dns.Msg)
	secondaryDNSChan := make(chan *dns.Msg)

	queryDNS := func(responseChan chan *dns.Msg, useSecondary bool) {
		upstreamDNS := server.config.PrimaryDNS
		if useSecondary {
			upstreamDNS = server.config.SecondaryDNS
		}

		response, err := resolve(request, upstreamDNS, network)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"operation": "resolve",
				"upstream":  upstreamDNS,
				"domain":    request.Question[0].Name,
			}).Error(err)
		}

		if response == nil || (!useSecondary && (response.Rcode != dns.RcodeSuccess || err != nil || server.isResponsePolluted(response))) {
			responseChan <- nil
			return
		}

		responseChan <- response
	}

	go queryDNS(primaryDNSChan, false)
	go queryDNS(secondaryDNSChan, true)

	go func() {
		time.Sleep(2 * time.Second)
		primaryDNSChan <- nil
		secondaryDNSChan <- nil
	}()

	response := <-primaryDNSChan
	if response != nil {
		server.setResponseCache(response, network)
		return response, server.config.PrimaryDNS, nil
	}

	response = <-secondaryDNSChan
	if response == nil {
		response = createServerFailureResponse(request)
	}
	if response.Rcode == dns.RcodeSuccess {
		server.setResponseCache(response, network)
	}
	return response, server.config.SecondaryDNS, nil
}

func createServerFailureResponse(request *dns.Msg) *dns.Msg {
	failureResponse := &dns.Msg{}
	failureResponse.SetRcode(request, dns.RcodeServerFailure)
	return failureResponse
}

func resolve(request *dns.Msg, upstreamDNS string, network string) (*dns.Msg, error) {
	resolvedRequest := request.Copy()
	resolvedRequest.Id = dns.Id()

	client := &dns.Client{Net: network}
	response, _, err := client.Exchange(resolvedRequest, upstreamDNS)

	return response, err
}

func (server *DNServer) isResponsePolluted(response *dns.Msg) bool {
	if containsARecord(response) {
		chinaIP := containsChinaIP(response)
		server.chinaDomainCache.Set(response.Question[0].Name, chinaIP)
		return !chinaIP
	}

	china, found := server.chinaDomainCache.Get(response.Question[0].Name)
	if found {
		return !china.(bool)
	}
	return false
}

func containsARecord(response *dns.Msg) bool {
	for _, rr := range append(append(response.Answer, response.Ns...), response.Extra...) {
		if _, ok := rr.(*dns.A); ok {
			return true
		}
	}
	return false
}

func containsChinaIP(response *dns.Msg) bool {
	for _, rr := range append(append(response.Answer, response.Ns...), response.Extra...) {
		if aRecord, ok := rr.(*dns.A); ok {
			ip := aRecord.A.String()
			if isChinaIP(ip) {
				return true
			}
		}
	}
	return false
}

func isChinaIP(ip string) bool {
	record, err := geoIPDB.Country(net.ParseIP(ip))
	if err != nil {
		logger.Error(err)
		return false
	}
	return record.Country.IsoCode == "CN"
}

func generateCacheKey(request *dns.Msg, network string) string {
	query := request.Question[0]
	key := query.Name + "_" + dns.TypeToString[query.Qtype]
	if request.RecursionDesired {
		key += "_RD"
	} else {
		key += "_NORD"
	}
	key += "_" + network
	return key
}

func (server *DNServer) LookupCache(request *dns.Msg, network string) (*dns.Msg, bool) {
	key := generateCacheKey(request, network)
	cachedItem, found := server.responseCache.Get(key)
	if found {
		cacheEntry := cachedItem.(cacheEntry)
		elapsed := time.Since(cacheEntry.putin).Seconds()

		responseCopy := cacheEntry.reply.Copy()
		needsUpdate := subtractTTL(responseCopy, int(elapsed))
		if needsUpdate {
			go server.refreshCache(request, network)
		}

		return responseCopy, true
	}
	return nil, false
}

func (server *DNServer) refreshCache(request *dns.Msg, network string) {
	response, upstream, _ := server.LookupNetwork(request, network)
	logEntry := logger.WithFields(logrus.Fields{
		"action":    "refreshCache",
		"domain":    request.Question[0].Name,
		"queryType": dns.TypeToString[request.Question[0].Qtype],
		"upstream":  upstream,
		"status":    dns.RcodeToString[response.Rcode],
	})

	if response.Rcode != dns.RcodeSuccess {
		logEntry.Warn()
	} else {
		logEntry.Info()
	}
}

func (server *DNServer) setResponseCache(response *dns.Msg, network string) {
	key := generateCacheKey(response, network)
	server.responseCache.Set(key, cacheEntry{
		putin: time.Now(),
		reply: response,
	})
}

type cacheEntry struct {
	putin time.Time
	reply *dns.Msg
}

func subtractTTL(response *dns.Msg, deltaSeconds int) bool {
	updateNeeded := false

	updateTTL := func(rrs []dns.RR) {
		for _, rr := range rrs {
			newTTL := int(rr.Header().Ttl) - deltaSeconds
			if newTTL <= 0 {
				newTTL = minimumTTL
				updateNeeded = true
			}
			rr.Header().Ttl = uint32(newTTL)
		}
	}

	updateTTL(response.Answer)
	updateTTL(response.Ns)
	updateTTL(response.Extra)

	return updateNeeded
}

func (server *DNServer) LookupHosts(request *dns.Msg) (*dns.Msg, bool) {
	// TODO: Implement host lookup logic
	return nil, false
}
