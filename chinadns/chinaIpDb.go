package chiandns

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const ipListURL = "https://raw.githubusercontent.com/kiddin9/china_ip_list/main/china_ip_list.txt"

func loadChinaIPs(url string, chinaIPs *[][]int) error {
	cidrs, err := fetchCIDRList(url)
	if err != nil {
		return err
	}

	for _, cidr := range cidrs {
		if cidr == "" {
			continue
		}
		start, end := parseCIDR(cidr)
		*chinaIPs = append(*chinaIPs, []int{start, end})
	}

	return nil
}

func fetchCIDRList(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return strings.Split(string(data), "\n"), nil
}

func parseCIDR(cidr string) (int, int) {
	var a, b, c, d, mask int
	fmt.Sscanf(cidr, "%d.%d.%d.%d/%d", &a, &b, &c, &d, &mask)
	base := toInt(a, b, c, d)
	start := base & (^(1<<(32-mask) - 1))
	end := start + (1 << (32 - mask)) - 1
	return start, end
}

func toInt(a, b, c, d int) int {
	return a<<24 | b<<16 | c<<8 | d
}
