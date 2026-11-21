package config

import (
	"flag"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env       string       `yaml:"env"`
	GarageCFG GarageConfig `yaml:"garage"`
	HTTP      HTTPConfig   `yaml:"http"`
}

type HTTPConfig struct {
	MaxFileBytes    int64 `yaml:"max_file_bytes"`
	MaxRequestBytes int64 `yaml:"max_request_bytes"`
	MaxImagePixels  int64 `yaml:"max_image_pixels" env-default:"16000000"`
}

type GarageConfig struct {
	BaseEndpoint string `yaml:"base_endpoint"`
	Region       string `yaml:"region"`
	Bucket       string `yaml:"bucket" env-default:"default-bucket"`
	UsePathStyle bool   `yaml:"use_path_style"`
}

func MustLoad() *Config {
	path := fetchConfigPath()
	if path == "" {
		panic("config path is empty")
	}

	return MustLoadByPath(path)
}

func MustLoadByPath(configPath string) *Config {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		panic("config file does not exist" + configPath)
	}
	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}

	return &cfg
}

func fetchConfigPath() string {
	var res string
	flag.StringVar(&res, "config", "", "path to config file")
	flag.Parse()
	if res == "" {
		res = os.Getenv("CONFIG_PATH")
	}
	return res
}
