package main

import (
	"log"
	"os"
	"strconv"
	"time"

	vt "github.com/lexfrei/vk2tg/internal/pkg/vk2tg"
)

const period = 10 * time.Second

func main() {
	logger := log.New(os.Stdout, "VK2TG: ", log.Ldate|log.Ltime|log.Lshortfile)

	user, err := strconv.Atoi(os.Getenv("V2T_TG_USER"))
	if err != nil {
		logger.Fatalln("Invalid TG user ID: " + os.Getenv("V2T_TG_USER"))
	}

	vtClient := vt.NewVTClient(
		os.Getenv("V2T_TG_TOKEN"),
		os.Getenv("V2T_VK_TOKEN"),
		user,
		period).WithLogger(logger).WithConfig("config.yaml")

	err = vtClient.LoadConfig()
	if err != nil {
		logger.Fatalln(err)
	}
	err = vtClient.Start()
	if err != nil {
		logger.Fatalln(err)
	}

	vtClient.Wait()
}
