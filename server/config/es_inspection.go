package config

type EsInspection struct {
	TaskInterval     string `mapstructure:"task-interval" json:"task-interval" yaml:"task-interval"`
	AliSlowThreshold int    `mapstructure:"ali-slow-threshold" json:"ali-slow-threshold" yaml:"ali-slow-threshold"`
	HwSlowThreshold  int    `mapstructure:"hw-slow-threshold" json:"hw-slow-threshold" yaml:"hw-slow-threshold"`
	InspectionUrl    string `mapstructure:"inspection-url" json:"inspection-url" yaml:"inspection-url"`
	InspectionBearer string `mapstructure:"inspection-bearer" json:"inspection-bearer" yaml:"inspection-bearer"`
	SdBrand          string `mapstructure:"sd-brand" json:"sd-brand" yaml:"sd-brand"`
	JdBrand          string `mapstructure:"jd-brand" json:"jd-brand" yaml:"jd-brand"`
}
