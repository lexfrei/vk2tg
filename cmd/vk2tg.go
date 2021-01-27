package main

import (
	"fmt"
	"log"
	"regexp"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	vkapi "github.com/lexfrei/golang-vk-api"
)

var reSplitter = regexp.MustCompile(`(?m)[^А-Яа-я]`)
var keys = []string{
	"ката",
	"кинжа",
	"клинок",
	"меч",
	"ищ",
	"изготов",
	"сошьет",
	"сдела",
	"поиск",
	"пересыл",
}

func main() {
	ticker := time.NewTicker(1 * time.Minute)

	posts := make(chan vkapi.WallPost)

	vkClient, err := vkapi.NewVKClientWithToken(vkToken, nil, true)
	if err != nil {
		log.Fatalln(err)
	}

	tgClient, err := tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		log.Fatalln(err)
	}

	go vk2tg.WatchNewPosts(ticker.C, posts, vkClient)
	go sendToTG(posts, tgClient)
	fmt.Scanln()
}

func sendToTG(posts <-chan vkapi.WallPost, bot *tgbotapi.BotAPI) {
	for p := range posts {
		fmt.Println(p.Text)

	}

}
