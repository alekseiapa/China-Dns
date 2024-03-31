package chiandns

import (
	"github.com/miekg/dns"
	"net"
)

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
