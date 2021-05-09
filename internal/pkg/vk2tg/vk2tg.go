package vk2tg

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	vkapi "github.com/himidori/golang-vk-api"
	"github.com/pkg/errors"
	tb "gopkg.in/tucnak/telebot.v2"
	"gopkg.in/yaml.v3"
)

// Moscow
//nolint:gomnd
var zone = time.FixedZone("UTC+3", 3*60*60)

type VTClinent struct {
	config     *config
	tgClient   *tb.Bot
	vkClient   *vkapi.VKClient
	LastUpdate time.Time
	StartTime  time.Time
	WG         *sync.WaitGroup
	ticker     *time.Ticker
	chVKPosts  chan *vkapi.WallPost
	logger     *log.Logger
	stateFile  string
}

type config struct {
	TGUser       int           `yaml:"TGUser"`
	Paused       bool          `yaml:"Paused"`
	Silent       bool          `yaml:"Silent"`
	TGToken      string        `yaml:"TGToken"`
	VKToken      string        `yaml:"VKToken"`
	Period       time.Duration `yaml:"Period"`
	LastPostID   int           `yaml:"LastPostID"`
	LastPostDate int64         `yaml:"LastPostDate"`
}

func NewVTClient(tgToken, vkToken string, tgRecepient int, period time.Duration) *VTClinent {
	c := new(VTClinent)
	c.config = new(config)
	c.config.TGToken = tgToken
	c.config.VKToken = vkToken
	c.config.TGUser = tgRecepient
	c.chVKPosts = make(chan *vkapi.WallPost)
	c.WG = &sync.WaitGroup{}
	c.config.Silent = false
	c.config.Paused = false
	c.StartTime = time.Now()
	c.config.Period = period
	c.ticker = time.NewTicker(period)
	c.logger = log.New(ioutil.Discard, "vk2tg: ", log.Ldate|log.Ltime|log.Lshortfile)
	return c
}

func (c *VTClinent) WithLogger(logger *log.Logger) *VTClinent {
	c.logger = logger
	return c
}

func (c *VTClinent) WithConfig(path string) *VTClinent {
	c.stateFile = path
	return c
}

func (c *VTClinent) Start() error {
	c.logger.Println("Starting...")
	var err error

	c.vkClient, err = vkapi.NewVKClientWithToken(c.config.VKToken, nil, true)
	if err != nil {
		return errors.Wrap(err, "Can't longin to VK")
	}

	c.tgClient, err = tb.NewBot(tb.Settings{
		Token:  c.config.TGToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return errors.Wrap(err, "Can't longin to TG")
	}

	c.tgClient.Handle("/status",
		func(m *tb.Message) {
			msg := fmt.Sprintf("I'm fine\nLast post date:\t%s\nReceived in:\t%s\nUptime:\t%s\nPaused:\t%t\nSound:\t%t",
				time.Unix(c.config.LastPostDate, 0).In(zone).Format(time.RFC822),
				c.LastUpdate.In(zone).Format(time.RFC822),
				time.Since(c.StartTime).Round(time.Second),
				c.config.Paused,
				!c.config.Silent,
			)
			c.sendMessage(m.Sender, msg)
		})

	c.tgClient.Handle("/pause",
		func(m *tb.Message) {
			if !c.config.Paused {
				c.Pause()
				c.sendMessage(m.Sender, "Paused! Send /pause to continue")
			} else {
				c.Resume()
				c.sendMessage(m.Sender, "Unpaused! Send /pause to stop")
			}
		})

	c.tgClient.Handle("/mute",
		func(m *tb.Message) {
			if !c.config.Silent {
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
	c.config.Paused = true
}

func (c *VTClinent) Resume() {
	c.logger.Println("Watcher unpaused")
	c.ticker.Reset(c.config.Period)
	c.config.Paused = false
}

func (c *VTClinent) Mute() {
	c.config.Silent = true
}

func (c *VTClinent) Unmute() {
	c.config.Silent = false
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

		if w.Posts[0].ID == c.config.LastPostID {
			c.logger.Printf("No new posts found")
			continue
		}

		for i := len(w.Posts) - 1; i >= 0; i-- {
			c.logger.Printf("Post %d: Processing", w.Posts[i].ID)

			if c.config.LastPostID > w.Posts[i].ID {
				c.logger.Printf("Post %d: Not a new post, skipped", w.Posts[i].ID)
				continue
			} else {
				c.logger.Printf("Post %d: Selected as latest", w.Posts[i].ID)
				c.config.LastPostDate = w.Posts[i].Date
				c.config.LastPostID = w.Posts[i].ID
				fmt.Println(c.SaveConfig())
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
			_, err := c.tgClient.SendAlbum(&tb.User{ID: c.config.TGUser}, album)
			if err != nil {
				c.logger.Printf("Can't send album: %s\n", err)
			}
		}

		_, err := c.tgClient.Send(
			&tb.User{ID: c.config.TGUser},
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
				DisableNotification: c.config.Silent,
			},
		)
		if err != nil {
			c.logger.Printf("Can't send message: %s\n", err)
		}
		ch, err := c.tgClient.ChatByID(strconv.Itoa(c.config.TGUser))
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

func (c *VTClinent) LoadConfig() error {
	_, err := os.Stat(c.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "Can't load config file:")
	}

	rawConfig, err := ioutil.ReadFile(c.stateFile)
	if err != nil {
		return errors.Wrap(err, "Can't read config file:")
	}

	return yaml.Unmarshal(rawConfig, c.config)
}

func (c *VTClinent) SaveConfig() error {
	b, err := yaml.Marshal(c.config)
	if err != nil {
		log.Fatal(err)
	}

	return ioutil.WriteFile(c.stateFile, b, 0600)
}
