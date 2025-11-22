package link

import (
	"bytes"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

// Link represents a network link.
type Link struct {
	Ifindex   int      `json:"ifindex,omitempty"`
	Link      string   `json:"link,omitempty"`
	Ifname    string   `json:"ifname,omitempty"`
	Flags     []string `json:"flags,omitempty"`
	Mtu       int      `json:"mtu,omitempty"`
	Qdisc     string   `json:"qdisc,omitempty"`
	Operstate string   `json:"operstate,omitempty"`
	Linkmode  string   `json:"linkmode,omitempty"`
	Group     string   `json:"group,omitempty"`
	LinkType  string   `json:"link_type,omitempty"`
	Address   string   `json:"address,omitempty"`
	Broadcast string   `json:"broadcast,omitempty"`
	Stats64   struct {
		Rx struct {
			Bytes      int `json:"bytes,omitempty"`
			Packets    int `json:"packets,omitempty"`
			Errors     int `json:"errors,omitempty"`
			Dropped    int `json:"dropped,omitempty"`
			OverErrors int `json:"over_errors,omitempty"`
			Multicast  int `json:"multicast,omitempty"`
		} `json:"rx,omitempty"`
		Tx struct {
			Bytes         int `json:"bytes,omitempty"`
			Packets       int `json:"packets,omitempty"`
			Errors        int `json:"errors,omitempty"`
			Dropped       int `json:"dropped,omitempty"`
			CarrierErrors int `json:"carrier_errors,omitempty"`
			Collisions    int `json:"collisions,omitempty"`
		} `json:"tx,omitempty"`
	} `json:"stats64,omitempty"`
}

type links []Link

// NewBuilder returns Link struct.
func NewBuilder(jsonOutput bytes.Buffer) (*Link, error) {
	var link links

	if len(jsonOutput.Bytes()) == 0 {
		klog.V(100).Info("Empty json output")

		return nil, fmt.Errorf("empty json output")
	}

	err := json.Unmarshal(jsonOutput.Bytes(), &link)
	if err != nil {
		klog.V(100).Infof("json unmarshalling failed: %v", err)

		return nil, fmt.Errorf("json unmarshalling failed: %w", err)
	}

	if len(link) < 1 {
		klog.V(100).Infof("no links to process found")

		return nil, fmt.Errorf("no links to process found")
	}

	if len(link) > 1 {
		klog.V(100).Infof("failed to process more than 1 link")

		return nil, fmt.Errorf("failed to process more than 1 link")
	}

	return &link[0], nil
}

// GetRxByte returns number of unicast bytes received on link.
func (l *Link) GetRxByte() int {
	return l.Stats64.Rx.Bytes
}

// NewListBuilder returns Link struct.
func NewListBuilder(jsonOutput bytes.Buffer) ([]Link, error) {
	var linkList links

	if len(jsonOutput.Bytes()) == 0 {
		klog.V(100).Info("Empty json output")

		return nil, fmt.Errorf("empty json output")
	}

	err := json.Unmarshal(jsonOutput.Bytes(), &linkList)
	if err != nil {
		klog.V(100).Infof("json unmarshalling failed: %v", err)

		return nil, fmt.Errorf("json unmarshalling failed: %w", err)
	}

	if len(linkList) < 1 {
		klog.V(100).Infof("no links to process found")

		return nil, fmt.Errorf("no links to process found")
	}

	return linkList, nil
}
