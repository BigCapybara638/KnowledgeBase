package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DataDir, PublicURL, ListenAddr string
	Dev                            bool
	TrustedProxies                 []net.IPNet
}

func Load() (Config, error) {
	c := Config{DataDir: os.Getenv("KB_DATA_DIR"), PublicURL: strings.TrimRight(os.Getenv("KB_PUBLIC_URL"), "/"), ListenAddr: os.Getenv("KB_LISTEN_ADDR")}
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	c.Dev, _ = strconv.ParseBool(os.Getenv("KB_DEV"))
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return c, errors.New("KB_PUBLIC_URL must be an absolute origin without path or credentials")
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(c.Dev && u.Scheme == "http" && local) {
		return c, errors.New("HTTPS is required; local HTTP requires KB_DEV=true")
	}
	for item := range strings.SplitSeq(os.Getenv("KB_TRUSTED_PROXY_CIDRS"), ",") {
		if item = strings.TrimSpace(item); item != "" {
			_, n, e := net.ParseCIDR(item)
			if e != nil {
				return c, e
			}
			c.TrustedProxies = append(c.TrustedProxies, *n)
		}
	}
	return c, nil
}
