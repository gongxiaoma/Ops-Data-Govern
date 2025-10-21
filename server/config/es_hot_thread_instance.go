package config

type EsHotThreadInstance struct {
	EsTag       string `mapstructure:"es-tag" json:"es-tag" yaml:"es-tag"`
	InstanceID  string `mapstructure:"instance-id" json:"instance-id" yaml:"instance-id"`
	Endpoint    string `mapstructure:"endpoint" json:"endpoint" yaml:"endpoint"`
	Username    string `mapstructure:"username" json:"username" yaml:"username"`
	Password    string `mapstructure:"password" json:"password" yaml:"password"`
	Timeout     int    `mapstructure:"timeout" json:"timeout" yaml:"timeout"`
	MaxRetries  int    `mapstructure:"max-retries" json:"max-retries" yaml:"max-retries"`
	EnableSniff bool   `mapstructure:"enable-sniff" json:"enable-sniff" yaml:"enable-sniff"`
	HotInterval string `mapstructure:"hot-interval" json:"hot-interval" yaml:"hot-interval"`
	HotThreads  int    `mapstructure:"hot-threads" json:"hot-threads" yaml:"hot-threads"`
	HotType     string `mapstructure:"hot-type" json:"hot-type" yaml:"hot-type"`
}
