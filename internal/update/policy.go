package update

import (
	"os"

	"github.com/yetone/magpie/internal/distribution"
)

// Policy identifies the immutable download channel of this package. A local
// brand override never turns a public package into an enterprise distribution.
type Policy struct {
	DistributionID string `json:"distributionId,omitempty"`
	Mode           string `json:"mode"`
	Channel        string `json:"channel,omitempty"`
	FeedURL        string `json:"feedUrl,omitempty"`
}

func CurrentPolicy() Policy { return policyFor(distribution.Current()) }

func policyFor(p distribution.Profile) Policy {
	if p.DistributionID != "" {
		policy := Policy{DistributionID: p.DistributionID, Mode: p.Updates.Mode, Channel: p.Updates.Channel}
		if p.Updates.Mode == "platform" {
			policy.FeedURL = p.Updates.FeedURL
		}
		return policy
	}
	feed := os.Getenv("MAGPIE_UPDATE_FEED")
	if feed == "" {
		feed = Site + "/api/latest"
	}
	return Policy{Mode: "public", FeedURL: feed}
}
