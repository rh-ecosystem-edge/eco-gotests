package nad

// IPAMStatic returns static ipam type.
func IPAMStatic() *IPAM {
	return &IPAM{Type: "static"}
}

// IPAMWhereAbouts returns WhereAbout ipam type.
func IPAMWhereAbouts(ipRange, gateway string) *IPAM {
	if ipRange == "" {
		return nil
	}

	if gateway == "" {
		return nil
	}

	return &IPAM{Type: "whereabouts", IPRanges: []IPRanges{{Range: ipRange, Gateway: gateway}}}
}

// WhereAboutsAppendRange returns WhereAbout ipam type with additional address range.
func WhereAboutsAppendRange(ipam *IPAM, ipRange, gateway string) *IPAM {
	if ipRange == "" {
		return nil
	}

	if gateway == "" {
		return nil
	}

	ipam.IPRanges = append(ipam.IPRanges, IPRanges{Range: ipRange, Gateway: gateway})

	return ipam
}
