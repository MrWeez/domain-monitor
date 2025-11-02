package configuration

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	whoisparser "github.com/likexian/whois-parser"
)

// RDAP domain response structure (simplified, only fields we need)
type rdapDomain struct {
	ObjectClassName string          `json:"objectClassName"`
	Handle          string          `json:"handle"`
	LDHName         string          `json:"ldhName"`
	Nameservers     []rdapNameserver `json:"nameservers"`
	Status          []string        `json:"status"`
	Entities        []rdapEntity    `json:"entities"`
	Events          []rdapEvent     `json:"events"`
	PublicIDs       []rdapPublicID  `json:"publicIds"`
	Links           []rdapLink      `json:"links"`
}

type rdapNameserver struct {
	ObjectClassName string      `json:"objectClassName"`
	LDHName         string      `json:"ldhName"`
	Links           []rdapLink  `json:"links"`
}

type rdapEntity struct {
	ObjectClassName string      `json:"objectClassName"`
	Handle          string      `json:"handle"`
	VCardArray      []interface{} `json:"vcardArray"`
	Roles           []string    `json:"roles"`
}

type rdapEvent struct {
	EventAction string    `json:"eventAction"`
	EventDate   string    `json:"eventDate"` // RDAP uses ISO8601 strings for dates
}

type rdapPublicID struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

type rdapLink struct {
	Value string `json:"value"`
	Rel   string `json:"rel"`
	Href  string `json:"href"`
	Type  string `json:"type"`
}

type rdapError struct {
	ErrorCode    int      `json:"errorCode"`
	Title        string   `json:"title"`
	Description  []string `json:"description"`
}

// QueryRDAP queries RDAP servers for domain information
func QueryRDAP(fqdn string) (whoisparser.WhoisInfo, error) {
	// Extract TLD from FQDN
	parts := strings.Split(fqdn, ".")
	if len(parts) < 2 {
		return whoisparser.WhoisInfo{}, fmt.Errorf("invalid domain: %s", fqdn)
	}
	tld := parts[len(parts)-1]

	// Try to get RDAP server URL from bootstrap service
	rdapServerURL, err := getRDAPServerFromBootstrap(tld)
	if err != nil {
		log.Printf("⚠️ Failed to get RDAP server from bootstrap for %s: %s", tld, err)
		// Fallback to common RDAP servers
		rdapServerURL = getFallbackRDAPServer(tld)
	}

	// Query RDAP server
	domainURL := fmt.Sprintf("%s/domain/%s", rdapServerURL, fqdn)
	log.Printf("🔍 Querying RDAP: %s", domainURL)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	resp, err := client.Get(domainURL)
	if err != nil {
		return whoisparser.WhoisInfo{}, fmt.Errorf("RDAP query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		var rdapErr rdapError
		if err := json.Unmarshal(bodyBytes, &rdapErr); err == nil && rdapErr.Title != "" {
			return whoisparser.WhoisInfo{}, fmt.Errorf("RDAP error: %s", rdapErr.Title)
		}
		return whoisparser.WhoisInfo{}, fmt.Errorf("RDAP query returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return whoisparser.WhoisInfo{}, fmt.Errorf("failed to read RDAP response: %w", err)
	}

	// Parse RDAP response
	var rdapDomainResp rdapDomain
	if err := json.Unmarshal(bodyBytes, &rdapDomainResp); err != nil {
		return whoisparser.WhoisInfo{}, fmt.Errorf("failed to parse RDAP response: %w", err)
	}

	// Convert RDAP response to whoisparser.WhoisInfo
	return convertRDAPToWhoisInfo(rdapDomainResp, fqdn), nil
}

// getRDAPServerFromBootstrap queries ICANN's RDAP bootstrap service
func getRDAPServerFromBootstrap(tld string) (string, error) {
	// ICANN bootstrap service uses dns.json for domain queries
	bootstrapURL := "https://data.iana.org/rdap/dns.json"
	
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	resp, err := client.Get(bootstrapURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bootstrap returned status %d", resp.StatusCode)
	}

	var bootstrapData struct {
		Services [][]interface{} `json:"services"`
	}
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(bodyBytes, &bootstrapData); err != nil {
		return "", err
	}

	// Find RDAP server URL for the TLD
	for _, service := range bootstrapData.Services {
		if len(service) >= 2 {
			tlds, ok := service[0].([]interface{})
			if !ok {
				continue
			}
			for _, t := range tlds {
				if tStr, ok := t.(string); ok && tStr == tld {
					servers, ok := service[1].([]interface{})
					if !ok || len(servers) == 0 {
						continue
					}
					// Return first RDAP server URL
					if serverURL, ok := servers[0].(string); ok {
						// Remove trailing slash if present
						return strings.TrimSuffix(serverURL, "/"), nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no RDAP server found for TLD %s", tld)
}

// getFallbackRDAPServer returns fallback RDAP servers for common TLDs
func getFallbackRDAPServer(tld string) string {
	// Common RDAP servers
	fallbackServers := map[string]string{
		"dev": "https://rdap.org",
		"com": "https://rdap.org",
		"net": "https://rdap.org",
		"org": "https://rdap.org",
		"io":  "https://rdap.org",
	}

	if server, ok := fallbackServers[strings.ToLower(tld)]; ok {
		return server
	}

	// Default fallback
	return "https://rdap.org"
}

// convertRDAPToWhoisInfo converts RDAP response to whoisparser.WhoisInfo format
func convertRDAPToWhoisInfo(rdap rdapDomain, fqdn string) whoisparser.WhoisInfo {
	domain := &whoisparser.Domain{
		ID:          rdap.Handle,
		Domain:      fqdn,
		Status:      rdap.Status,
		NameServers: []string{},
	}

	// Extract nameservers
	for _, ns := range rdap.Nameservers {
		if ns.LDHName != "" {
			domain.NameServers = append(domain.NameServers, ns.LDHName)
		}
	}

	// Extract registrar from entities (look for registrar role)
	var registrar *whoisparser.Contact
	for _, entity := range rdap.Entities {
		isRegistrar := false
		for _, role := range entity.Roles {
			if role == "registrar" {
				isRegistrar = true
				registrar = &whoisparser.Contact{
					ID: entity.Handle,
				}
				break
			}
		}
		
		if isRegistrar {
			// Try to extract registrar name from vCard
			if len(entity.VCardArray) > 1 {
				if vcardItems, ok := entity.VCardArray[1].([]interface{}); ok {
					for _, item := range vcardItems {
						if itemArray, ok := item.([]interface{}); ok && len(itemArray) >= 4 {
							if itemType, ok := itemArray[0].(string); ok {
								// Look for "fn" (full name) field in vCard
								if itemType == "fn" {
									if name, ok := itemArray[3].(string); ok && name != "" {
										registrar.Name = name
										break
									}
								}
								// Also check for "org" (organization) field as fallback
								if itemType == "org" && registrar.Name == "" {
									if org, ok := itemArray[3].(string); ok && org != "" {
										registrar.Organization = org
										registrar.Name = org
									}
								}
							}
						}
					}
				}
			}
			// If we still don't have a name, use the handle
			if registrar.Name == "" && entity.Handle != "" {
				registrar.Name = entity.Handle
			}
			break
		}
	}

	// Extract dates from events
	// RDAP dates are in ISO8601 format (RFC3339)
	for _, event := range rdap.Events {
		if event.EventDate == "" {
			continue
		}
		
		eventTime, err := time.Parse(time.RFC3339, event.EventDate)
		if err != nil {
			// Try alternative format if RFC3339 fails
			eventTime, err = time.Parse("2006-01-02T15:04:05Z", event.EventDate)
			if err != nil {
				log.Printf("⚠️ Warning: Failed to parse RDAP date '%s' for event '%s': %s", event.EventDate, event.EventAction, err)
				continue
			}
		}
		
		// Create pointer to time
		eventTimePtr := &eventTime
		
		switch event.EventAction {
		case "registration":
			domain.CreatedDate = eventTime.Format(time.RFC3339)
			domain.CreatedDateInTime = eventTimePtr
		case "expiration":
			domain.ExpirationDate = eventTime.Format(time.RFC3339)
			domain.ExpirationDateInTime = eventTimePtr
		case "last changed", "last update":
			domain.UpdatedDate = eventTime.Format(time.RFC3339)
			domain.UpdatedDateInTime = eventTimePtr
		}
	}

	// Set default values if critical dates are missing
	if domain.ExpirationDateInTime == nil {
		// If expiration date is missing, this is a critical error
		// but we'll still return the info we have
		log.Printf("⚠️ Warning: No expiration date found in RDAP response for %s", fqdn)
	}

	return whoisparser.WhoisInfo{
		Domain:    domain,
		Registrar: registrar,
	}
}
