package config

import (
	"flag"
	"os"
)

type Config struct {
	Addr                 string
	DBConnection         string
	AccrualSystemAddress string

	Timeout    int
	NumWorkers int
}

func NewConfig() *Config {
	return &Config{
		Addr:                 "localhost:8080",
		DBConnection:         "",
		AccrualSystemAddress: "", // TODO
		Timeout:              15,
		NumWorkers:           15,
	}
}

func Init(c *Config) {
	if val, exist := os.LookupEnv("RUN_ADDRESS"); exist {
		c.Addr = val
	}
	if val, exist := os.LookupEnv("DATABASE_URI"); exist {
		c.DBConnection = val
	}
	if val, exist := os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS"); exist {
		c.AccrualSystemAddress = val
	}

	flag.StringVar(&c.Addr, "a", c.Addr, "HTTP-server startup address and port")
	flag.StringVar(&c.DBConnection, "d", c.DBConnection, "database connection address")
	flag.StringVar(&c.AccrualSystemAddress, "r", c.AccrualSystemAddress, "accrual calculation system address")

	flag.Parse()
}
