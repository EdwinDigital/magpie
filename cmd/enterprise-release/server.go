package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/yetone/magpie/internal/enterprisebuild"
	"github.com/yetone/magpie/internal/enterprisedistribution"
)

func serve(ctx context.Context, listen, access string, o enterprisebuild.Options, keyPath, keyID, baseURL string) error {
	fi, err := os.Stat(access)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("access configuration must be private (mode 0600)")
	}
	b, err := os.ReadFile(access)
	if err != nil {
		return err
	}
	var principals []enterprisedistribution.Principal
	if err := json.Unmarshal(b, &principals); err != nil {
		return err
	}
	cfg := enterprisedistribution.Config{Store: o.Output, BaseURL: baseURL, Principals: principals, BuildOptions: o, SigningKeyID: keyID}
	if keyPath != "" {
		key, err := enterprisebuild.ReadPrivateKey(keyPath)
		if err != nil {
			return err
		}
		cfg.SigningKey = key
	}
	s, err := enterprisedistribution.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer s.Close()
	h := &http.Server{Addr: listen, Handler: s, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: time.Minute, IdleTimeout: time.Minute}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = h.Shutdown(shutdown)
		case <-done:
		}
	}()
	defer close(done)
	fmt.Println("Enterprise distribution API:", listen, "(HTTPS ingress and enterprise authentication integration required)")
	err = h.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
