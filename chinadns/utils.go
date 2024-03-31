package chiandns

import (
	"github.com/miekg/dns"
	"strconv"
	"strings"
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

func IPToInteger(ip string) int {
	var parts = strings.Split(ip, ".")
	if len(parts) < 4 {
		return 0
	}

	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[1])
	c, _ := strconv.Atoi(parts[2])
	d, _ := strconv.Atoi(parts[3])

	return (a << 24) | (b << 16) | (c << 8) | d
}

func isChinaIP(ip string) bool {
	ipInt := IPToInteger(ip)
	left := 0
	right := len(chinaIPs) - 1

	for left <= right {
		mid := left + (right-left)/2
		if ipInt < chinaIPs[mid][0] {
			right = mid - 1
		} else if ipInt > chinaIPs[mid][1] {
			left = mid + 1
		} else {
			return true
		}
	}

	return false
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
