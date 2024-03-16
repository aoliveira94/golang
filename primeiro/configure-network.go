package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Data struct {
	Network NetworkInfo `json:"network"`
}

type NetworkInfo struct {
	Server     Range    `json:"server"`
	POS        Range    `json:"pos"`
	KDS        Range    `json:"kds"`
	FAILOVER   Range    `json:"failover"`
	Nameserver []string `json:"nameservers"`
	Subnet     Subnet   `json:"subnet"`
	Gateway    string   `json:"gateway"`
	DHCP       bool     `json:"dhcp"`
}

type Range struct {
	FROM int `json:"from"`
	TO   int `json:"to"`
}

type Subnet struct {
	IP   string `json:"ip"`
	Mask int    `json:"mask"`
}

func getApiJson() []Data {
	store := os.Getenv("STORE")
	apiUrl := "url da sua api. realizando um get no json"
	url := apiUrl + store
	method := "GET"
	apiKey := "seu token de acesso"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		handleError("Error creating request:", err)
		return nil
	}
	req.Header.Set("X-API-KEY", apiKey)

	res, err := client.Do(req)
	if err != nil {
		handleError("Error making request:", err)
		return nil
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		handleError("Error reading response body:", err)
		return nil
	}

	var responseData []Data
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		handleError("Error parsing JSON:", err)
		return nil
	}

	return responseData
}

func handleError(msg string, err error) {
	fmt.Println(msg, err)
	os.Exit(1)
}

func getNetworkInfo() (NetworkInfo, error) {
	responseData := getApiJson()
	if len(responseData) == 0 {
		return NetworkInfo{}, fmt.Errorf("no data received from API")
	}
	return responseData[0].Network, nil
}

func getIPRange(network Range, subnetIP string) []string {
	var ips []string
	for i := network.FROM; i <= network.TO; i++ {
		ip := fmt.Sprintf("%s%d", subnetIP, i)
		if !checkIP(ip) {
			ips = append(ips, ip)
		}
	}
	return ips
}

func checkIP(ip string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", "1000", ip)
	case "darwin", "linux":
		cmd = exec.Command("ping", "-c", "1", "-W", "1", ip)
	default:
		fmt.Println("Unsupported operating system")
		return false
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false // IP unavailable or error
	}

	outputStr := string(output)
	return !strings.Contains(outputStr, "2 received")
}

func configureNetworkLinux(ipOff string, subnetMask int, nameservers []string, gateway string) {
	netplanConfig := fmt.Sprintf(`network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: no
      addresses: ["%s/%d"]
      gateway4: "%s"
      nameservers:
        addresses: [%s]
`, ipOff, subnetMask, gateway, strings.Join(nameservers, ", "))

	netplanFilePath := "/etc/netplan/99-custom.yaml"

	err := os.WriteFile(netplanFilePath, []byte(netplanConfig), 0644)
	if err != nil {
		handleError("Error writing Netplan configuration file:", err)
	}

	cmd := exec.Command("netplan", "apply")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		handleError("Error applying Netplan configuration:", err)
	}

	fmt.Println("Static IP configured successfully on Linux using Netplan.")
}

func configureNetworkWindows(ipOff string, subnetMask int, nameservers []string, gateway string) {
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("New-NetIPAddress -InterfaceAlias Ethernet -IPAddress %s -PrefixLength %d -DefaultGateway %s", ipOff, subnetMask, gateway))
	err := cmd.Run()
	if err != nil {
		handleError("Error configuring static IP on Windows:", err)
	}

	dnsCmd := "Set-DnsClientServerAddress -InterfaceAlias Ethernet -ServerAddresses " + strings.Join(nameservers, ",")
	cmdSetDNS := exec.Command("powershell", "-Command", dnsCmd)
	err = cmdSetDNS.Run()
	if err != nil {
		handleError("Error configuring DNS servers on Windows:", err)
	}

	fmt.Println("Static IP configured successfully on Windows using PowerShell.")
}

func checkDhcp() {
	networkInfo, err := getNetworkInfo()
	if err != nil {
		handleError("Error getting network information:", err)
	}

	if networkInfo.DHCP {
		fmt.Println("DHCP configured")
		os.Exit(1)
	}

	subnetIP := networkInfo.Subnet.IP[:9] + "."
	var ipOff string
	typeNode := os.Getenv("TYPENODE")

	switch typeNode {
	case "server":
		ipOff = getIPRange(networkInfo.Server, subnetIP)[0]
	case "pos":
		ipOff = getIPRange(networkInfo.POS, subnetIP)[0]
	case "kds":
		ipOff = getIPRange(networkInfo.KDS, subnetIP)[0]
	case "failover":
		ipOff = getIPRange(networkInfo.FAILOVER, subnetIP)[0]
	default:
		handleError("Unknown machine type:", nil)
	}

	if ipOff != "" {
		fmt.Println("-----------Configuring static IP")
		fmt.Println("-----------Addresses:", ipOff+"/", +networkInfo.Subnet.Mask)
		fmt.Println("-----------Name Servers:", networkInfo.Nameserver)
	} else {
		fmt.Println("No unavailable IP found.")
	}

	configureNetwork(ipOff, networkInfo.Subnet.Mask, networkInfo.Nameserver, networkInfo.Gateway)
}

func configureNetwork(ipOff string, subnetMask int, nameservers []string, gateway string) {
	switch runtime.GOOS {
	case "linux":
		configureNetworkLinux(ipOff, subnetMask, nameservers, gateway)
	case "windows":
		configureNetworkWindows(ipOff, subnetMask, nameservers, gateway)
	default:
		fmt.Println("Unsupported operating system")
		os.Exit(1)
	}
}

func main() {
	checkDhcp()
}
