package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func SearchUrl(input string) (string, error) {
	regex := regexp.MustCompile(`https?:\/\/(?:www\.)?([-\d\w.]{2,256}[\d\w]{2,6}\b)*(\/[?\/\d\w\=+&#.-]*)*`)
	m := regex.FindStringSubmatch(input)
	if m != nil {
		return m[0], nil
	}
	return "", fmt.Errorf("url format not supported")
}

type Bulk struct {
	UrlToshorten string `json:"short_url"`
	Duration     int    `json:"duration"`
}
type Arr struct {
	Urls []Bulk `json:"data"`
}
type Shortend struct {
	Url         *string `json:"short_url,omitempty"`
	Duration    *int    `json:"duration,omitempty"`
	Originalurl *string `json:"original_url,omitempty"`
	Status      *bool   `json:"key_exists,omitempty"`
	Err         *string `json:"error,omitempty"`
}
type Resp struct {
	Short []Shortend `json:"data"`
}
type BasicERR struct {
	Invalid_body *string `json:"invalid_body,omitempty"`
	Too_many     *string `json:"too_many,omitempty"`
	No_urls      *string `json:"no_urls,omitempty"`
}

func GenrateUid(input string) (string, string) {
	bytes := []byte(input)
	encoded := base64.StdEncoding.EncodeToString(bytes)
	uuid := sha256.Sum256([]byte(input))
	start := len(uuid) / 2
	finegrain := uuid[start : start+6]
	return fmt.Sprintf("%x", finegrain), encoded
}
func CheckID(input string, client *redis.Client) (string, error) {
	regex := regexp.MustCompile(`^[A-Za-z0-9]{7}$`)
	m := regex.FindStringSubmatch(input)
	if m != nil {
		id := m[0]
		checked_id, checked_err := client.Get(ctx, id).Result()
		if checked_err != nil {
			return "", fmt.Errorf("key not found")
		}
		decodedBytes, err := base64.StdEncoding.DecodeString(checked_id)
		if err != nil {
			return "", fmt.Errorf("error while getting url")
		}
		return string(decodedBytes), nil
	}
	return "", fmt.Errorf("id format is not supported")
}

func main() {
	app := fiber.New()
	var env_err = godotenv.Load()
	if env_err != nil {
		log.Fatal("Error loading .env file")
	}
	api_host := os.Getenv("HOST")
	db_url := os.Getenv("REDIS")
	con, con_err := redis.ParseURL(db_url)
	if con_err != nil {
		log.Fatal("Error  while connecting  to database")
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
		AllowMethods: "GET,POST",
	}))
	client := redis.NewClient(con)
	app.Use(limiter.New())
	app.Get("/api", func(c *fiber.Ctx) error {
		uri := c.Query("url")
		duration, duration_err := strconv.Atoi(c.Query("dur"))
		if duration_err != nil {
			err := "invalid params structure"
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Originalurl: nil, Err: &err, Status: nil})
		}
		if duration > 3600 {
			err := "High Duration (max 3600)!"
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Originalurl: nil, Err: &err, Status: nil})
		}
		search, search_err := SearchUrl(uri)
		if search_err != nil {
			err := search_err.Error()
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Originalurl: nil, Err: &err, Status: nil})
		}
		u, fullstr := GenrateUid(search)
		status := client.SetNX(ctx, u[:7], fullstr, time.Duration(duration)*time.Second).Val()
		c.Response().Header.Add("Cache-Control", "max-age="+fmt.Sprint(duration))
		shorted := api_host + "/" + u[:7]
		stat := !status
		return c.Status(fiber.StatusOK).JSON(Shortend{Duration: &duration, Url: &shorted, Status: &stat})
	})
	app.Get("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		res, res_err := CheckID(id, client)
		if res_err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(res_err.Error())
		}
		return c.Redirect(res)
	})
	app.Post("/api/bulk", func(c *fiber.Ctx) error {
		req_body := c.Body()
		obj := string(req_body)
		var data Arr
		parse_err := json.Unmarshal([]byte(obj), &data)
		if parse_err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("unsupported data structure")
		}
		if body_parse_err := c.BodyParser(&data); body_parse_err != nil {
			err := "invalid data structure"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{Invalid_body: &err, Too_many: nil, No_urls: nil})
		}
		if len(data.Urls) == 0 {
			err := "no urls"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{Invalid_body: nil, Too_many: nil, No_urls: &err})
		}
		if len(data.Urls) > 50 {
			err := "max allowed urls is 50"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{Invalid_body: nil, Too_many: &err, No_urls: nil})
		}
		var res_obj []Shortend
		ca := make(chan Shortend)
		wg := sync.WaitGroup{}
		go func(data Arr, ca chan Shortend) {
			for _, ur := range data.Urls {
				origin := ur.UrlToshorten
				duration := ur.Duration
				wg.Add(1)
				go func() {
					defer wg.Done()
					if duration > 3600 {
						err := "High Duration (max 3600)!"
						ca <- Shortend{Originalurl: &origin, Duration: &duration, Url: nil, Err: &err, Status: nil}
						return
					}
					if duration == 0 {
						duration += 60
					}
					search, search_err := SearchUrl(origin)
					if search_err != nil {
						err := search_err.Error()
						ca <- Shortend{Originalurl: &origin, Duration: nil, Url: nil, Err: &err, Status: nil}
						return
					}
					uid, fullstr := GenrateUid(search)
					shotrend_url := api_host + "/" + uid[:7]
					status := client.SetNX(ctx, uid[:7], fullstr, time.Duration(duration)*time.Second).Val()
					stat := !status
					ca <- Shortend{Originalurl: &origin, Duration: &duration, Url: &shotrend_url, Err: nil, Status: &stat}
				}()
			}
			wg.Wait()
			close(ca)
		}(data, ca)
		for res := range ca {
			res_obj = append(res_obj, res)
		}
		return c.Status(fiber.StatusCreated).JSON(Resp{Short: res_obj})
	})
	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "8000"
	}
	host := "0.0.0.0:" + port
	if listen_err := app.Listen(host); listen_err != nil {
		log.Fatalf("error : %s", listen_err.Error())
	}
}
