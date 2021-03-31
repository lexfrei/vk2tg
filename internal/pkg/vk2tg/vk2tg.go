package vk2tg

import (
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	vkapi "github.com/himidori/golang-vk-api"
	"github.com/pkg/errors"
	tb "github.com/tucnak/telebot"
)

// Moscow
//nolint:gomnd
var zone = time.FixedZone("UTC+3", 3*60*60)

type VTClinent struct {
	tgUser       int
	Paused       bool
	Silent       bool
	tgToken      string
	vkToken      string
	Period       time.Duration
	tgClient     *tb.Bot
	vkClient     *vkapi.VKClient
	LastPostID   int
	LastPostDate int64
	LastUpdate   time.Time
	StartTime    time.Time
	WG           *sync.WaitGroup
	ticker       *time.Ticker
	chVKPosts    chan *vkapi.WallPost
	logger       *log.Logger
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
	c.logger = log.New(ioutil.Discard, "vk2tg: ", log.Ldate|log.Ltime|log.Lshortfile)
	return c
}

//nolint:lll
func NewVTClientWithLogger(tgToken, vkToken string, tgRecepient int, period time.Duration, logger *log.Logger) *VTClinent {
	c := NewVTClient(tgToken, vkToken, tgRecepient, period)
	c.logger = logger
	return c
}

func (c *VTClinent) Start() error {
	c.logger.Println("Starting...")
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
			msg := fmt.Sprintf("I'm fine\nLast post date:\t%s\nReceived in:\t%s\nUptime:\t%s\nPaused:\t%t\nSound:\t%t",
				time.Unix(c.LastPostDate, 0).In(zone).Format(time.RFC822),
				c.LastUpdate.In(zone).Format(time.RFC822),
				time.Since(c.StartTime).Round(time.Second),
				c.Paused,
				!c.Silent,
			)
			c.sendMessage(m.Sender, msg)
		})

	c.tgClient.Handle("/pause",
		func(m *tb.Message) {
			if !c.Paused {
				c.Pause()
				c.sendMessage(m.Sender, "Paused! Send /pause to continue")
			} else {
				c.Resume()
				c.sendMessage(m.Sender, "Unpaused! Send /pause to stop")
			}
		})

	c.tgClient.Handle("/mute",
		func(m *tb.Message) {
			if !c.Silent {
				c.Mute()
				c.sendMessage(m.Sender, "Muted! Send /mute to go loud")
			} else {
				c.Unmute()
				c.sendMessage(m.Sender, "Unmuted! Send /mute to go silent")
			}
		})

	go c.tgClient.Start()
	//nolint:gomnd
	c.WG.Add(2)
	go c.VKWatcher()
	go c.TGSender()

	return nil
}

func (c *VTClinent) Pause() {
	c.logger.Println("Watcher paused")
	c.ticker.Stop()
	c.Paused = true
}

func (c *VTClinent) Resume() {
	c.logger.Println("Watcher unpaused")
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
		c.logger.Println("Fetching new posts")
		w, err := c.vkClient.WallGet("cosplay_second", 10, nil)
		if err != nil {
			c.logger.Printf("failed to fetch posts: %s", err)
			continue
		}

		if w.Posts[0].ID == c.LastPostID {
			c.logger.Printf("No new posts found")
			continue
		}

		for i := len(w.Posts) - 1; i >= 0; i-- {
			c.logger.Printf("Post %d: Processing", w.Posts[i].ID)

			if c.LastPostID > w.Posts[i].ID {
				c.logger.Printf("Post %d: Not a new post, skipped", w.Posts[i].ID)
				continue
			} else {
				c.logger.Printf("Post %d: Selected as latest", w.Posts[i].ID)
				c.LastPostDate = w.Posts[i].Date
				c.LastPostID = w.Posts[i].ID
			}

			if !strings.Contains(w.Posts[i].Text, "#поиск") {
				c.logger.Printf("Post %d: Post does not contain required substring, skipping", w.Posts[i].ID)
				continue
			}

			c.logger.Printf("Post %d: Sending to TG", w.Posts[i].ID)
			c.chVKPosts <- w.Posts[i]
		}
	}
}

func (c *VTClinent) TGSender() {
	defer c.WG.Done()
	defer c.logger.Println("Sender: done")
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
				c.logger.Printf("Can't send album: %s\n", err)
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
			c.logger.Printf("Can't send message: %s\n", err)
		}
		ch, err := c.tgClient.ChatByID(strconv.Itoa(c.tgUser))
		if err != nil {
			c.logger.Printf("Error on fetching user info: %s", err)
		}
		c.logger.Printf("Post %d: Sent to %s", p.ID, ch.FirstName)
	}
}

func (c *VTClinent) sendMessage(u *tb.User, str string) {
	_, err := c.tgClient.Send(u, str)
	if err != nil {
		c.logger.Printf("Error on sending message: %s", err)
	}
}
