package config

import (
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv(files ...string) error {
	if len(files) == 0 {
		return godotenv.Load()
	}
	for _, f := range files {
		if err := godotenv.Load(f); err != nil {
			if _, ok := err.(*os.PathError); ok {
				continue
			}
			return err
		}
	}
	return nil
}

func LoadEnvOverride(files ...string) error {
	if len(files) == 0 {
		return godotenv.Overload()
	}
	for _, f := range files {
		if err := godotenv.Overload(f); err != nil {
			if _, ok := err.(*os.PathError); ok {
				continue
			}
			return err
		}
	}
	return nil
}
