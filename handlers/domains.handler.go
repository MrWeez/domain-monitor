package handlers

import (
	"errors"
	"log"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/nwesterhausen/domain-monitor/configuration"
	"github.com/nwesterhausen/domain-monitor/service"
	"github.com/nwesterhausen/domain-monitor/views/domains"
)

type DomainHandler struct {
	DomainService ApiDomainService
	WhoisService  *service.ServicesWhois
}

func NewDomainHandler(ds ApiDomainService, ws *service.ServicesWhois) *DomainHandler {
	return &DomainHandler{
		DomainService: ds,
		WhoisService:  ws,
	}
}

// Get the HTML for the domain card inner content from 'fqdn' parameter
func (h *DomainHandler) GetCard(c echo.Context) error {
	fqdn := c.Param("fqdn")
	if len(fqdn) == 0 {
		return errors.New("Invalid domain to fetch (FQDN required)")
	}

	domain, err := h.DomainService.GetDomain(fqdn)
	if err != nil {
		return err
	}
	card := domains.DomainCard(domain)
	return View(c, card)
}

// Get the HTML for all the domain cards
func (h *DomainHandler) GetCards(c echo.Context) error {
	domainList, err := h.DomainService.GetDomains()
	if err != nil {
		return err
	}

	// Get sort parameter from query string (default: expiration_date)
	sortBy := c.QueryParam("sort")
	if sortBy == "" {
		sortBy = "expiration_date"
	}

	// Sort domains based on WHOIS data if available
	if h.WhoisService != nil && sortBy != "name" {
		domainList = h.sortDomainsWithWhois(domainList, sortBy)
	} else if sortBy == "name" {
		sort.Slice(domainList, func(i, j int) bool {
			return domainList[i].Name < domainList[j].Name
		})
	}

	cards := domains.DomainCards(domainList, sortBy)
	return View(c, cards)
}

// sortDomainsWithWhois sorts domains by expiration date, creation date, or name using WHOIS data
func (h *DomainHandler) sortDomainsWithWhois(domainList []configuration.Domain, sortBy string) []configuration.Domain {
	type domainWithWhois struct {
		domain configuration.Domain
		whois  *configuration.WhoisCache
	}

	domainsWithData := make([]domainWithWhois, 0, len(domainList))
	for _, domain := range domainList {
		whois, err := h.WhoisService.GetWhois(domain.FQDN)
		if err != nil {
			// If WHOIS data not available, still include domain but with nil whois
			domainsWithData = append(domainsWithData, domainWithWhois{domain: domain, whois: nil})
			continue
		}
		domainsWithData = append(domainsWithData, domainWithWhois{domain: domain, whois: &whois})
	}

	// Sort based on sortBy parameter
	sort.Slice(domainsWithData, func(i, j int) bool {
		wi, wj := domainsWithData[i].whois, domainsWithData[j].whois

		switch sortBy {
		case "expiration_date":
			// Sort by expiration date (ascending - soonest first)
			if wi == nil || wi.WhoisInfo.Domain == nil || wi.WhoisInfo.Domain.ExpirationDateInTime == nil {
				return false // Put domains without expiration date at the end
			}
			if wj == nil || wj.WhoisInfo.Domain == nil || wj.WhoisInfo.Domain.ExpirationDateInTime == nil {
				return true
			}
			return wi.WhoisInfo.Domain.ExpirationDateInTime.Before(*wj.WhoisInfo.Domain.ExpirationDateInTime)
		case "creation_date":
			// Sort by creation date (descending - newest first)
			if wi == nil || wi.WhoisInfo.Domain == nil || wi.WhoisInfo.Domain.CreatedDateInTime == nil {
				return false
			}
			if wj == nil || wj.WhoisInfo.Domain == nil || wj.WhoisInfo.Domain.CreatedDateInTime == nil {
				return true
			}
			return wi.WhoisInfo.Domain.CreatedDateInTime.After(*wj.WhoisInfo.Domain.CreatedDateInTime)
		case "name":
			return domainsWithData[i].domain.Name < domainsWithData[j].domain.Name
		default:
			return false
		}
	})

	// Extract sorted domains
	sortedDomains := make([]configuration.Domain, len(domainsWithData))
	for i, dw := range domainsWithData {
		sortedDomains[i] = dw.domain
	}

	return sortedDomains
}

// Get HTML for domain list as tbody
func (h *DomainHandler) GetListTbody(c echo.Context) error {
	domainList, err := h.DomainService.GetDomains()
	if err != nil {
		return err
	}
	list := domains.DomainListingTbody(domainList)
	return View(c, list)
}

// Add a domain and return an updated tbody
func (h *DomainHandler) PostNewDomain(c echo.Context) error {
	var domain configuration.Domain
	if err := c.Bind(&domain); err != nil {
		return err
	}

	log.Printf("🆕 Adding domain: %+v\n", domain)

	_, err := h.DomainService.CreateDomain(domain)
	if err != nil {
		return err
	}

	return h.GetListTbody(c)
}

// Delete a domain and return an updated tbody
func (h *DomainHandler) DeleteDomain(c echo.Context) error {
	fqdn := c.Param("fqdn")
	if len(fqdn) == 0 {
		return errors.New("invalid domain to delete (FQDN required)")
	}

	log.Printf("🙅 Deleting domain: %s\n", fqdn)

	err := h.DomainService.DeleteDomain(fqdn)
	if err != nil {
		return err
	}

	return h.GetListTbody(c)
}

// Get the HTML for the domain edit form
func (h *DomainHandler) GetEditDomain(c echo.Context) error {
	fqdn := c.Param("fqdn")
	if len(fqdn) == 0 {
		return errors.New("invalid domain to edit (FQDN required)")
	}

	domain, err := h.DomainService.GetDomain(fqdn)
	if err != nil {
		return err
	}

	log.Printf("🛰️ Editing domain: %+v\n", domain)

	return View(c, domains.DomainTableRowInput(strings.ReplaceAll(domain.FQDN, ".", "_"), domain))
}

// Update a domain and return an updated tbody
func (h *DomainHandler) PostUpdateDomain(c echo.Context) error {
	var domain configuration.Domain
	if err := c.Bind(&domain); err != nil {
		return err
	}

	log.Printf("🛰️ Updating domain: %+v\n", domain)

	err := h.DomainService.UpdateDomain(domain)
	if err != nil {
		return err
	}

	// Get the updated domain from storage to ensure we have the latest data
	updatedDomain, err := h.DomainService.GetDomain(domain.FQDN)
	if err != nil {
		return err
	}

	return h.GetListDomainRow(updatedDomain, c)
}

// Get the HTML for a single domain row
func (h *DomainHandler) GetListDomainRow(domain configuration.Domain, c echo.Context) error {
	row := domains.DomainTableRow(domain)
	return View(c, row)
}
