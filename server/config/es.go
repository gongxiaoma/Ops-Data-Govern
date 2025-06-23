package config

type Es struct {
	ClusterNode []string `mapstructure:"cluster-node" json:"cluster-node" yaml:"cluster-node"`
	UserName    string   `mapstructure:"username" json:"username" yaml:"username"`
	PassWord    string   `mapstructure:"password" json:"password" yaml:"password"`
}
