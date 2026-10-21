package main

import "testing"

func TestIdentityBrokerFlagMapsToConfig(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"--mock"}, false},
		{[]string{"--mock", "--enable-identity-broker"}, true},
		{[]string{"--mock", "--enable-identity-broker=false"}, false},
	} {
		cfg, err := parseFlags(c.args)
		if err != nil {
			t.Fatalf("parseFlags(%v): %v", c.args, err)
		}
		if got := appConfig(cfg, nil, observability{}).IdentityBroker; got != c.want {
			t.Errorf("parseFlags(%v): IdentityBroker = %v, want %v", c.args, got, c.want)
		}
	}
}
