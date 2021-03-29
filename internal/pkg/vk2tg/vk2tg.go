package vk2tg

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	vkapi "github.com/himidori/golang-vk-api"
	"github.com/pkg/errors"
	tb "github.com/tucnak/telebot"
)

type VTClinent struct {
	tgUser     int
	Paused     bool
	Silent     bool
	tgToken    string
	vkToken    string
	Period     time.Duration
	tgClient   *tb.Bot
	vkClient   *vkapi.VKClient
	LastPost   *vkapi.WallPost
	LastUpdate time.Time
	StartTime  time.Time
	WG         *sync.WaitGroup
	ticker     *time.Ticker
	chVKPosts  chan *vkapi.WallPost
}

func NewVTClient(tgToken, vkToken string, tgRecepient int, period time.Duration) *VTClinent {
	c := new(VTClinent)
	c.tgToken = tgToken
	c.vkToken = vkToken
	c.tgUser = tgRecepient
	c.chVKPosts = make(chan *vkapi.WallPost)
	c.WG = &sync.WaitGroup{}
	c.Silent = false
	c.Paused = false
	c.StartTime = time.Now()
	c.Period = period
	c.ticker = time.NewTicker(period)
	return c
}

func (c *VTClinent) Start() error {
	var err error

	c.vkClient, err = vkapi.NewVKClientWithToken(c.vkToken, nil, true)
	if err != nil {
		return errors.Wrap(err, "Can't longin to VK")
	}

	c.tgClient, err = tb.NewBot(tb.Settings{
		Token:  c.tgToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return errors.Wrap(err, "Can't longin to TG")
	}

	c.tgClient.Handle("/status",
		func(m *tb.Message) {
			msg := fmt.Sprintf("I'm fine\nLast post date:\t%s\nRecived in:\t%s\nUptime:\t%s\nPaused:\t%t\nSound:\t%t",
				time.Unix(c.LastPost.Date, 0).In(time.FixedZone("UTC+3", 3*60*60)).Format(time.RFC822),
				c.LastUpdate.In(time.FixedZone("UTC+3", 3*60*60)).Format(time.RFC822),
				time.Since(c.StartTime).Round(time.Second),
				c.Paused,
				!c.Silent,
			)
			_, err = c.tgClient.Send(m.Sender, msg)
			if err != nil {
				log.Printf("Error on sending status: %s", err)
			}
		})

	c.tgClient.Handle("/pause",
		func(m *tb.Message) {
			if c.Paused {
				c.Pause()
				c.tgClient.Send(m.Sender, "Paused! Send /pause to continue")
			} else {
				c.Resume()
				c.tgClient.Send(m.Sender, "Unpaused! Send /pause to stop")
			}
		})

	c.tgClient.Handle("/mute", func(m *tb.Message) {
		if err != nil {
			log.Printf("Error on fetching user info: %s", err)
		}
		if c.Silent {
			c.Mute()
			c.tgClient.Send(m.Sender, "Muted! Send /mute to go loud")
		} else {
			c.Unmute()
			c.tgClient.Send(m.Sender, "Unmuted! Send /mute to go silent")
		}
	})

	c.tgClient.Start()
	c.WG.Add(2)
	go c.VKWatcher()
	go c.TGSender()

	return nil
}

func (c *VTClinent) Pause() {
	c.ticker.Stop()
	c.Paused = true
}

func (c *VTClinent) Resume() {
	c.ticker.Reset(c.Period)
	c.Paused = false
}

func (c *VTClinent) Mute() {
	c.Silent = true
}

func (c *VTClinent) Unmute() {
	c.Silent = false
}

func (c *VTClinent) Wait() {
	c.WG.Wait()
}

func (c *VTClinent) VKWatcher() {
	defer c.WG.Done()
	for range c.ticker.C {
		c.LastUpdate = time.Now()
		log.Println("Fetching new posts")
		w, err := c.vkClient.WallGet("cosplay_second", 10, nil)
		if err != nil {
			log.Printf("failed to fetch posts: %s", err)
			continue
		}

		if w.Posts[0].ID == c.LastPost.ID {
			log.Printf("No new posts found")
			continue
		}

		for i := len(w.Posts) - 1; i >= 0; i-- {
			log.Printf("Post %d: Processing", w.Posts[i].ID)

			if c.LastPost.ID > w.Posts[i].ID {
				log.Printf("Post %d: Not a new post, skipped", w.Posts[i].ID)
				continue
			} else {
				log.Printf("Post %d: Selected as latest", w.Posts[i].ID)
				c.LastPost = w.Posts[i]
			}

			if !strings.Contains(w.Posts[i].Text, "#поиск") {
				log.Printf("Post %d: Post does not contain required substring, skipping", w.Posts[i].ID)
				continue
			}

			log.Printf("Post %d: Sending to TG", w.Posts[i].ID)
			c.chVKPosts <- w.Posts[i]
		}
	}
}

func (c *VTClinent) TGSender() {
	defer c.WG.Done()
	defer log.Println("Sender: done")
	for p := range c.chVKPosts {
		var album tb.Album
		for _, a := range p.Attachments {
			if a.Type == "photo" {
				var maxSize int
				var url string
				for _, size := range a.Photo.Sizes {
					if maxSize < size.Width*size.Height {
						maxSize = size.Width * size.Height
						url = size.Url
					}
				}
				album = append(album, &tb.Photo{
					File: tb.FromURL(url),
				})
			}
		}

		if len(album) > 0 {
			_, err := c.tgClient.SendAlbum(&tb.User{ID: c.tgUser}, album)
			if err != nil {
				log.Printf("Can't send album: %s\n", err)
			}
		}

		_, err := c.tgClient.Send(
			&tb.User{ID: c.tgUser},
			p.Text,
			&tb.SendOptions{
				ReplyTo: &tb.Message{},
				ReplyMarkup: &tb.ReplyMarkup{
					InlineKeyboard: [][]tb.InlineButton{
						{
							tb.InlineButton{
								Text: "🌎 К посту",
								URL:  "https://vk.com/wall-57692133_" + strconv.Itoa(p.ID),
							},
							tb.InlineButton{
								Text: "✍️ Написать",
								URL:  "vk.com/write" + strconv.Itoa(p.SignerID),
							},
						},
					},
				},
				DisableNotification: c.Silent,
			},
		)
		if err != nil {
			log.Printf("Can't send message: %s\n", err)
		}
		ch, err := c.tgClient.ChatByID(strconv.Itoa(c.tgUser))
		if err != nil {
			log.Printf("Error on fetching user info: %s", err)
		}
		log.Printf("Message sent to %s", ch.FirstName)
	}
}
