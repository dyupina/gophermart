package config

import (
	"flag"
	"fmt"
	"gophermart/cmd/gophermart/utils"
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
		Addr:                 ":8081", // Don't edit
		DBConnection:         "",
		AccrualSystemAddress: ":8080",
		Timeout:              15,
		NumWorkers:           15,
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

	fmt.Printf(">>>c.Addr %s\n", c.Addr)                                 // TODO remove
	fmt.Printf(">>>c.DBConnection %s\n", c.DBConnection)                 // TODO remove
	fmt.Printf(">>>c.AccrualSystemAddress %s\n", c.AccrualSystemAddress) // TODO remove

	if c.DBConnection == "" {
		return fmt.Errorf("set DATABASE_URI env variable")
	}

	// Регистрация информации о вознаграждении за товар (POST /api/goods) @@@
	utils.RegisterRewards(c.AccrualSystemAddress)

	return nil
}
