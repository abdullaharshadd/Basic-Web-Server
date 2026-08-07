package config

import "github.com/spf13/viper"

type Config struct {
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	Port        string `mapstructure:"PORT"`
	JWTSecret   string `mapstructure:"JWT_SECRET"`
}

func Load() (*Config, error) {
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
