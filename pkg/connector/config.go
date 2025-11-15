// mautrix-irc - A Matrix-IRC puppeting bridge.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	_ "embed"
	"strings"

	up "go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"
	"maunium.net/go/mautrix/id"
)

//go:embed example-config.yaml
var ExampleConfig string

type NetworkConfig struct {
	DisplayName string              `yaml:"displayname"`
	AvatarURL   id.ContentURIString `yaml:"avatar_url"`
	ExternalURL string              `yaml:"external_url"`
	Address     string              `yaml:"address"`
	TLS         bool                `yaml:"tls"`
	CTCP        bool                `yaml:"ctcp"`
	Name        string              `yaml:"-"`
}

type IdentdConfig struct {
	Address      string `yaml:"address"`
	StrictRemote bool   `yaml:"strict_remote"`
}

type Config struct {
	Networks map[string]*NetworkConfig `yaml:"networks"`
	Identd   IdentdConfig              `yaml:"identd"`
}

func (ic *IRCConnector) GetConfig() (example string, data any, upgrader up.Upgrader) {
	return ExampleConfig, &ic.Config, &up.StructUpgrader{
		SimpleUpgrader: up.SimpleUpgrader(upgradeConfig),
		Blocks:         [][]string{},
		Base:           ExampleConfig,
	}
}

type umConfig Config

func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	err := node.Decode((*umConfig)(c))
	if err != nil {
		return err
	}
	return c.PostProcess()
}

func (c *Config) PostProcess() (err error) {
	for name, net := range c.Networks {
		if net.DisplayName == "" {
			net.DisplayName = name
		}
		if name != strings.ToLower(name) {
			c.Networks[strings.ToLower(name)] = net
			delete(c.Networks, name)
			name = strings.ToLower(name)
		}
		net.Name = name
	}
	return
}

func upgradeConfig(helper up.Helper) {
	helper.Copy(up.Map, "networks")
	helper.Copy(up.Str|up.Null, "identd.address")
	helper.Copy(up.Bool, "identd.strict_remote")
}
