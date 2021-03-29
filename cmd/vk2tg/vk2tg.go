package main

import (
	"log"
	"os"
	"strconv"
	"time"

	vt "github.com/lexfrei/vk2tg/internal/pkg/vk2tg"
)

func main() {
	var err error

	user, err := strconv.Atoi(os.Getenv("V2T_TG_USER"))
	if err != nil {
		log.Fatalln("Invalid TG user ID: " + os.Getenv("V2T_TG_USER"))
	}

	vtClient := vt.NewVTClient(
		os.Getenv("V2T_TG_TOKEN"),
		os.Getenv("V2T_VK_TOKEN"),
		user,
		60*time.Second,
	)

	vtClient.Start()

	vtClient.Wait()
}
