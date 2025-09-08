package config

// GlobalIPReplacementList defines global IP and network replacement rules.
type GlobalIPReplacementList struct {
	Net []GlobalNetReplacement `mapstructure:"net"`
	IP  []GlobalIPReplacement  `mapstructure:"ip"`
}

// GlobalNetReplacement defines a named group of network replacements.
type GlobalNetReplacement struct {
	Name string      `mapstructure:"name"`
	Nets []NetConfig `mapstructure:"nets"`
}

// GlobalIPReplacement defines a named group of IP replacements.
type GlobalIPReplacement struct {
	Name string     `mapstructure:"name"`
	IPs  []IPConfig `mapstructure:"ips"`
}

// NetConfig defines a network replacement rule.
type NetConfig struct {
	Source string `mapstructure:"source"`
	Target string `mapstructure:"target"`
	Mask   int    `mapstructure:"mask"`
}

// IPConfig defines an IP replacement rule.
type IPConfig struct {
	Source string `mapstructure:"source"`
	Target string `mapstructure:"target"`
}
