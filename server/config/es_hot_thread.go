package config

type EsHotThread struct {
	TaskInterval string                `mapstructure:"task-interval" json:"task-interval" yaml:"task-interval"`
	Aliyun       []EsHotThreadInstance `mapstructure:"aliyun" json:"aliyun" yaml:"aliyun"`
	Huawei       []EsHotThreadInstance `mapstructure:"huawei" json:"huawei" yaml:"huawei"`
}
