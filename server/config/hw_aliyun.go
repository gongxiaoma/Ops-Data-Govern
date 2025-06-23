package config

type HwyunEs struct {
	CipherKey      string   `mapstructure:"cipher-key" json:"cipher-key" yaml:"cipher-key"`
	HwyunKey       string   `mapstructure:"hwyun-key" json:"hwyun-key" yaml:"hwyun-key"`
	HwyunSecret    string   `mapstructure:"hwyun-secret" json:"hwyun-secret" yaml:"hwyun-secret"`
	Region         string   `mapstructure:"region" json:"region" yaml:"region"`
	ContextTimeout int      `mapstructure:"context-timeout" json:"context-timeout" yaml:"context-timeout"`
	TaskInterval   string   `mapstructure:"task-interval" json:"task-interval" yaml:"task-interval"`
	MaxConcurrency int      `mapstructure:"max-concurrency" json:"max-concurrency" yaml:"max-concurrency"`
	EsList         []string `mapstructure:"es-list" json:"es-list" yaml:"es-list"`
}
