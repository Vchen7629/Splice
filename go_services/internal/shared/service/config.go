package service

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

func LoadConfig[T any]() (*T, error) {
	if err := godotenv.Load("../.env"); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		log.Println("missing .env file")
	}
	var cfg T

	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}

	if err := validator.New().Struct(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}
