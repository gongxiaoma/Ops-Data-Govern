package config

type AliyunEs struct {
	CipherKey      string   `mapstructure:"cipher-key" json:"cipher-key" yaml:"cipher-key"`
	AliyunKey      string   `mapstructure:"aliyun-key" json:"aliyun-key" yaml:"aliyun-key"`
	AliyunSecret   string   `mapstructure:"aliyun-secret" json:"aliyun-secret" yaml:"aliyun-secret"`
	Region         string   `mapstructure:"region" json:"region" yaml:"region"`
	ContextTimeout int      `mapstructure:"context-timeout" json:"context-timeout" yaml:"context-timeout"`
	TaskInterval   string   `mapstructure:"task-interval" json:"task-interval" yaml:"task-interval"`
	MaxConcurrency int      `mapstructure:"max-concurrency" json:"max-concurrency" yaml:"max-concurrency"`
	BatchSize      int      `mapstructure:"batch-size" json:"batch-size" yaml:"batch-size"`
	MinutesBack    int64    `mapstructure:"minutes-back" json:"minutes-back" yaml:"minutes-back"`
	EsList         []string `mapstructure:"es-list" json:"es-list" yaml:"es-list"`
}
