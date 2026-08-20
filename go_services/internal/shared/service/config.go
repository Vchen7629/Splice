package service

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

func LoadConfig[T any]() (*T, error) {
	err := godotenv.Load("../.env")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		log.Println("missing .env file")
	}
	var cfg T

	err = envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
