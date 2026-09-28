package main

import (
	"fmt"
	"strings"
)

type EndpointDomain struct {
	Domain string
}

// CheckBuggy mirrors the original EndpointDomain.check() behavior.
func (ep *EndpointDomain) CheckBuggy(domain string) string {
	if domain == ep.Domain {
		return "Permitted"
	}
	return "NoMatch"
}

// CheckFixed mirrors the patched EndpointDomain.check() behavior.
func (ep *EndpointDomain) CheckFixed(domain string) string {
	d := strings.ToLower(domain)
	if !strings.HasSuffix(d, ".") {
		d += "."
	}

	if d == ep.Domain {
		return "Permitted"
	}
	return "NoMatch"
}

func main() {
	// Rule initialized by parseTypeDomain: lowercased with trailing dot
	rule := &EndpointDomain{Domain: "5ea1.playfabapi.com."}

	testCases := []string{
		"5ea1.playfabapi.com",   // Missing trailing dot (standard OS/app input)
		"5EA1.PLAYFABAPI.COM",   // Uppercase & missing trailing dot
		"5ea1.PlayFabApi.com.",  // Mixed case with trailing dot
	}

	fmt.Println("=== Portmaster Domain Matching Repro ===")
	fmt.Printf("Rule Definition: %q\n\n", rule.Domain)

	for _, input := range testCases {
		oldResult := rule.CheckBuggy(input)
		newResult := rule.CheckFixed(input)

		fmt.Printf("Input: %-25q -> Old: %-8s | Fixed: %s\n", input, oldResult, newResult)
	}

	fmt.Println("\nEvaluation Flow Simulation for '5ea1.playfabapi.com':")
	fmt.Println("-------------------------------------------------------")
	fmt.Printf("Old Engine:   Rule 1 (+ 5ea1.playfabapi.com) -> %s\n", rule.CheckBuggy("5ea1.playfabapi.com"))
	fmt.Println("              Rule 2 (- *)                   -> Denied (BLOCKED)")
	fmt.Println()
	fmt.Printf("Fixed Engine: Rule 1 (+ 5ea1.playfabapi.com) -> %s (ALLOWED)\n", rule.CheckFixed("5ea1.playfabapi.com"))
}