package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr                 string
	DBConnection         string
	AccrualSystemAddress string
	Timeout              int
	NumWorkers           int
	MaxRequestsPerMin    int
}

func NewConfig() *Config {
	return &Config{
		Addr:                 ":8081", // Don't edit
		DBConnection:         "",
		AccrualSystemAddress: ":8080",
		Timeout:              15,
		NumWorkers:           2,
		MaxRequestsPerMin:    240,
	}
}

func Init(c *Config) error {
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

	fmt.Printf("c.Addr %s\n", c.Addr)
	fmt.Printf("c.DBConnection %s\n", c.DBConnection)
	fmt.Printf("c.AccrualSystemAddress %s\n", c.AccrualSystemAddress)

	if c.DBConnection == "" {
		return errors.New("set DATABASE_URI env variable")
	}
	if c.AccrualSystemAddress == "" {
		return errors.New("set ACCRUAL_SYSTEM_ADDRESS env variable")
	}

	if !strings.HasPrefix(c.AccrualSystemAddress, "http://") {
		c.AccrualSystemAddress = "http://" + c.AccrualSystemAddress
	}

	return nil
}
