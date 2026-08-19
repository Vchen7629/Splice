//go:build unit

package test

type MockConfig struct {
	NatsURL    string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode   bool   `envconfig:"PROD_MODE" default:"false"`
	StorageURL string `envconfig:"STORAGE_URL" default:"http://localhost:8888"`
	HTTPPort   string `envconfig:"HTTP_PORT" default:"8080"`
}
