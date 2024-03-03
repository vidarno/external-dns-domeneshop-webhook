package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	domeneshop "github.com/vidarno/external-dns-domeneshop-webhook/internal/client"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type Provider struct {
	domeneshopClient *domeneshop.Client
}

func NewProvider(apiToken, apiSecret string) *Provider {
	provider := Provider{
		domeneshopClient: domeneshop.NewClient(apiToken, apiSecret),
	}

	return &provider
}

func (p *Provider) AdjustEndpoints(body io.ReadCloser) ([]byte, error) {
	var pve []*endpoint.Endpoint
	if err := json.NewDecoder(body).Decode(&pve); err != nil {
		return nil, errors.New("empty name")
	}

	// If we needed to adjust the endpoints, this would be a great place to do it
	// since we don't, currently we just return them as-is

	out, _ := json.Marshal(&pve)
	return out, nil

}
func (p *Provider) ApplyChanges(body io.ReadCloser) (error string) {
	var changes plan.Changes
	if err := json.NewDecoder(body).Decode(&changes); err != nil {
		return "StatusBadRequest"
	}
	fmt.Printf("requesting apply changes, create: %d , updateOld: %d, updateNew: %d, delete: %d",
		len(changes.Create), len(changes.UpdateOld), len(changes.UpdateNew), len(changes.Delete))

	// TODO: Support dry-run
	// Log total requested and total processed to debug failures more easily

	for _, record := range changes.Create {
		// Is the domain valid ?
		domainZone, ok := getDomainZone(p.domeneshopClient, record.DNSName)
		if !ok {
			fmt.Printf("Could not find appropriate domain, skip this record")
			continue
		}

		// Loop over all targets
		for _, target := range record.Targets {

			// Convert to Domeneshop Domain-struct
			dnsRecord := endpointToDnsRecord(domainZone, record, target)

			// Call appropriate Domeneshop-function
			ok := p.domeneshopClient.CreateRecord(domainZone, dnsRecord)
			if !ok {
				return "StatusInternalServerError"
			}

		}

	}

	for changeIndex, record := range changes.UpdateNew {

		oldRecord := changes.UpdateOld[changeIndex]
		if isSameEndpoint(record, oldRecord) {
			// Do nothing if there is no actual change
			continue
		}

		// Is the domain valid ?
		domainZone, ok := getDomainZone(p.domeneshopClient, record.DNSName)
		if !ok {
			fmt.Printf("Could not find appropriate domain, skip this record")
			continue
		}

		// Loop over all targets
		for targetIndex, target := range record.Targets {

			oldTarget := oldRecord.Targets[targetIndex]

			// Convert to Domeneshop Domain-structs
			oldDnsRecord := endpointToDnsRecord(domainZone, oldRecord, oldTarget)
			newDnsRecord := endpointToDnsRecord(domainZone, record, target)

			// Call appropriate Domeneshop-function
			ok := p.domeneshopClient.UpdateRecord(domainZone, oldDnsRecord, newDnsRecord)
			if !ok {
				return "StatusInternalServerError"
			}

		}

	}

	for _, record := range changes.Delete {
		// Is the domain valid ?
		domainZone, ok := getDomainZone(p.domeneshopClient, record.DNSName)
		if !ok {
			fmt.Printf("Could not find appropriate domain, skip this record")
			continue
		}

		// Loop over all targets
		for _, target := range record.Targets {

			// Convert to Domeneshop Domain-struct
			dnsRecord := endpointToDnsRecord(domainZone, record, target)

			// Call appropriate Domeneshop-function
			err := p.domeneshopClient.DeleteRecord(domainZone, dnsRecord)
			if err != nil {
				return "StatusInternalServerError"
			}

		}

	}

	return "StatusNoContent"

}

func (p *Provider) Records() []*endpoint.Endpoint {

	endpoints := make([]*endpoint.Endpoint, 0)

	// Get all domains
	domains, err := p.domeneshopClient.GetDomains()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Get all records for each domain
	for _, domain := range domains {
		records, err := p.domeneshopClient.GetRecords(domain.ID)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		for _, record := range records {
			fqdn := record.Host + "." + domain.Name
			endpoints = append(endpoints, endpoint.NewEndpointWithTTL(fqdn, record.Type, endpoint.TTL(record.TTL), record.Data))

		}

	}
	// TODO: use SupportedRecordType in provider-package for external-dns to filter records

	return endpoints
}

func getDomainZone(client *domeneshop.Client, DNSName string) (string, bool) {
	zone := DNSName
	for {
		parts := strings.Split(zone, ".")
		if len(parts) < 2 {
			// Didn't find a valid domain before reaching top-level-domain
			break
		}
		// Re-assemble the domain part
		domainPart := strings.Join(parts[1:], ".")
		domain, err := client.GetDomainByName(domainPart)
		if err != nil {
			continue
		}
		return domain.Name, true

	}
	// Failure
	return "", false
}

func endpointToDnsRecord(domainZone string, record *endpoint.Endpoint, target string) domeneshop.DNSRecord {
	var ttl int
	host := strings.Split(record.DNSName, "."+domainZone)[0]
	if int(record.RecordTTL) < 60 {
		// Default in Domeneshop API is 3600
		ttl = 3600
	} else {
		ttl = int(record.RecordTTL)
	}
	domain := domeneshop.DNSRecord{
		Host: host,
		Data: target,
		TTL:  ttl,
		Type: record.RecordType,
	}
	if record.RecordType == "MX" {
		// TODO: Do we split the target-string into priority and target?
	}

	return domain

}

func isSameEndpoint(a *endpoint.Endpoint, b *endpoint.Endpoint) bool {
	return a.DNSName == b.DNSName && a.RecordType == b.RecordType && a.RecordTTL == b.RecordTTL && a.Targets.Same(b.Targets)
}
