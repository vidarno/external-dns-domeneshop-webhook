package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

const apiURL string = "https://api.domeneshop.no/v0"
const version string = "0.0.1"

// This code was based on the cert-manager-webhook-domeneshop made by Domeneshop
// and extending it with more functions and support for other recordtypes than TXT
//
// https://api.domeneshop.no/docs/
type Client struct {
	APIToken  string
	APISecret string
	http      http.Client
}

// Domain JSON data structure
type Domain struct {
	Name           string   `json:"domain"`
	ID             int      `json:"id"`
	ExpiryDate     string   `json:"expiry_date"`
	Nameservers    []string `json:"nameservers"`
	RegisteredDate string   `json:"registered_date"`
	Registrant     string   `json:"registrant"`
	Renew          bool     `json:"renew"`
	Services       struct {
		DNS       bool   `json:"dns"`
		Email     bool   `json:"email"`
		Registrar bool   `json:"registrar"`
		Webhotel  string `json:"webhotel"`
	} `json:"services"`
	Status string
}

// DNSRecord JSON data structure
type DNSRecord struct {
	Data     string `json:"data"`
	Host     string `json:"host"`
	ID       int    `json:"id"`
	TTL      int    `json:"ttl"`
	Type     string `json:"type"`
	Priority string `json:"priority"`
}

// NewClient returns an instance of the Domeneshop API wrapper
func NewClient(apiToken, apiSecret string) *Client {
	client := Client{
		APIToken:  apiToken,
		APISecret: apiSecret,
		http:      http.Client{},
	}

	return &client
}

// GetDomainByName fetches the domain list and returns the Domain object
// for the matching domain.
func (c *Client) GetDomainByName(domain string) (*Domain, error) {
	var domains []Domain

	err := c.request("GET", "domains", nil, &domains)
	if err != nil {
		return nil, err
	}

	for _, d := range domains {
		if !d.Services.DNS {
			// Domains without DNS service cannot have DNS record added
			continue
		}
		if d.Name == domain {
			return &d, nil
		}
	}

	return nil, fmt.Errorf("failed to find matching domain name: %s", domain)
}

// getDNSRecordByHostDataType finds the first matching DNS record with the provided host, data and type
func (c *Client) getDNSRecordByHostDataType(domain Domain, host string, data string, recordTtype string) (*DNSRecord, error) {
	var records []DNSRecord

	err := c.request("GET", fmt.Sprintf("domains/%d/dns", domain.ID), nil, &records)
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.Host == host && r.Data == data && r.Type == recordTtype {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("failed to find record with host %s for domain %s", host, domain.Name)

}

// GetDomains fetches the domain list and returns the Domain object
// for the matching domain.
func (c *Client) GetDomains() ([]Domain, error) {
	var domains []Domain
	domains_list := make([]Domain, 0)

	err := c.request("GET", "domains", nil, &domains)
	if err != nil {
		return nil, err
	}

	for _, d := range domains {
		if !d.Services.DNS {
			// Domains without DNS service cannot have DNS record added
			continue
		}
		domains_list = append(domains_list, d)
	}
	if len(domains_list) > 0 {
		return domains_list, nil
	}

	return nil, fmt.Errorf("failed to find domains")
}

// GetRecords fetches the records for the specified domain
func (c *Client) GetRecords(domainId int) ([]DNSRecord, error) {
	var records []DNSRecord
	endpoint := "domains/" + strconv.Itoa(domainId) + "/dns"

	err := c.request("GET", endpoint, nil, &records)
	if err != nil {
		return nil, err
	}

	if len(records) > 0 {
		return records, nil
	}

	return nil, fmt.Errorf("failed to find records for specified domain")
}

func (c *Client) CreateRecord(domainZone string, record DNSRecord) bool {
	var err error
	domain, err := c.GetDomainByName(domainZone)
	if err != nil {
		return false
	}
	switch record.Type {
	case "A", "AAAA", "CNAME", "TXT":
		err = c.createSimpleRecord(domain, record)
	case "MX":
		err = c.createMXRecord(domain, record)
	default:
		// We can't handle other types at the moment
		return false
	}

	return err == nil

}

// DeleteRecord deletes the DNS record matching the provided host and data
func (c *Client) DeleteRecord(domainZone string, dnsRecord DNSRecord) error {

	domain, err := c.GetDomainByName(domainZone)
	if err != nil {
		return fmt.Errorf("failed to find domain with name %s", domainZone)
	}

	host := dnsRecord.Host
	data := dnsRecord.Data
	recordType := dnsRecord.Type
	var record *DNSRecord

	switch recordType {
	case "A", "AAAA", "CNAME", "TXT":
		record, err = c.getDNSRecordByHostDataType(*domain, host, data, recordType)
		if err != nil {
			return fmt.Errorf("failed to find record with data %s", data)
		}
	case "MX":
		// TODO: We can handle this
		return fmt.Errorf("we currently don't support MX-records")
	default:
		// We can't handle other types at the moment
		return fmt.Errorf("unsupported recordType")
	}

	return c.request("DELETE", fmt.Sprintf("domains/%d/dns/%d", domain.ID, record.ID), nil, nil)
}

// UpdateRecord updates the record matching oldDnsRecord with values from newDnsRecord
func (c *Client) UpdateRecord(domainZone string, oldDnsRecord, newDnsRecord DNSRecord) bool {

	domain, err := c.GetDomainByName(domainZone)
	if err != nil {
		// failed to find domain with name domainZone
		return false
	}

	recordType := oldDnsRecord.Type

	switch recordType {
	case "A", "AAAA", "CNAME", "TXT":
		record, err := c.getDNSRecordByHostDataType(*domain, oldDnsRecord.Host, oldDnsRecord.Data, recordType)
		if err != nil {
			// failed to find record with data oldDnsRecord.Data
			return false
		}
		err = c.updateSimpleRecord(domain, record, newDnsRecord)
		if err != nil {
			// failed to update record with data newDnsRecord.Data
			return false
		}

	case "MX":
		// TODO: We can handle this in the future, but not yet
		return false
	default:
		// We can't handle other types at the moment
		return false
	}

	return true
}

// request makes a request against the API with an optional body, and makes sure
// that the required Authorization header is set using `setBasicAuth`
func (c *Client) request(method string, endpoint string, reqBody []byte, v interface{}) error {

	var buf = bytes.NewBuffer(reqBody)

	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", apiURL, endpoint), buf)
	if err != nil {
		return err
	}

	req.SetBasicAuth(c.APIToken, c.APISecret)

	versionInfo := version

	req.Header.Set("User-Agent", fmt.Sprintf("external-dns-domeneshop-webhook/v"+versionInfo))

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode > 399 {
		return fmt.Errorf("API returned %s: %s", resp.Status, respBody)
	}

	if v != nil {
		return json.Unmarshal(respBody, &v)
	}
	return nil
}

func (c *Client) createSimpleRecord(domain *Domain, record DNSRecord) error {

	jsonRecord, err := json.Marshal(DNSRecord{
		Data: record.Data,
		Host: record.Host,
		TTL:  record.TTL,
		Type: record.Type,
	})

	if err != nil {
		return err
	}

	return c.request("POST", fmt.Sprintf("domains/%d/dns", domain.ID), jsonRecord, nil)
}

func (c *Client) createMXRecord(domain *Domain, record DNSRecord) error {

	jsonRecord, err := json.Marshal(DNSRecord{
		Data:     record.Data,
		Host:     record.Host,
		TTL:      record.TTL,
		Type:     "TXT",
		Priority: record.Priority,
	})

	if err != nil {
		return err
	}

	return c.request("POST", fmt.Sprintf("domains/%d/dns", domain.ID), jsonRecord, nil)
}

func (c *Client) updateSimpleRecord(domain *Domain, oldRecord *DNSRecord, newRecord DNSRecord) error {

	jsonRecord, err := json.Marshal(DNSRecord{
		Data: newRecord.Data,
		Host: newRecord.Host,
		TTL:  newRecord.TTL,
		Type: newRecord.Type,
	})

	if err != nil {
		return err
	}

	return c.request("PUT", fmt.Sprintf("domains/%d/dns/%d", domain.ID, oldRecord.ID), jsonRecord, nil)
}
