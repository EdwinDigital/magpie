package update

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/distribution"
)

func enterpriseRequestAuth(req *http.Request) error {
	return enterpriseRequestAuthFor(req, distribution.Current())
}

func enterpriseRequestAuthFor(req *http.Request, profile distribution.Profile) error {
	if profile.DistributionID == "" {
		return nil
	}
	feed, err := url.Parse(profile.Updates.FeedURL)
	if err != nil {
		return err
	}
	if req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Host, feed.Host) {
		return nil
	}
	path := filepath.Join(appdir.Config(), "enterprise-update-token")
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o077 != 0 || fi.Size() > 8192 {
		return fmt.Errorf("enterprise update credential must be a private file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(string(b))
	if token == "" || strings.ContainsAny(token, "\r\n\t ") {
		return fmt.Errorf("invalid enterprise update credential")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func enterpriseRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many update redirects")
	}
	if distribution.ID() == "" {
		return nil
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("enterprise update redirects require HTTPS")
	}
	feed, err := url.Parse(distribution.Current().Updates.FeedURL)
	if err != nil {
		return err
	}
	if !strings.EqualFold(req.URL.Host, feed.Host) {
		req.Header.Del("Authorization")
	}
	return nil
}
