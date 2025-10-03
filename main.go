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
	UrlToshorten string `json:"url"`
	Duration     int    `json:"duration"`
}
type Arr struct {
	Urls []Bulk `json:"data"`
}
type Shortend struct {
	Url        *string `json:"short_url,omitempty"`
	Duration   *int    `json:"duration,omitempty"`
	Orginalurl *string `json:"original_url,omitempty"`
	Status     *bool   `json:"key_exists,omitempty"`
	Err        *string `json:"error,omitempty"`
}
type Resp struct {
	Short []Shortend `json:"data"`
}
type BasicERR struct {
	Invalid_body *string `json:"body,omitempty"`
	Too_many     *string `json:"to_many,omitempty"`
	No_urls      *string `json:"no_url,omitempty"`
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
		ch, cherr := client.Get(ctx, id).Result()
		if cherr != nil {
			return "", fmt.Errorf("key not found")
		}
		decodedBytes, err := base64.StdEncoding.DecodeString(ch)
		if err != nil {
			return "", fmt.Errorf("error while getting url")
		}
		return string(decodedBytes), nil
	}
	return "", fmt.Errorf("id format is not supported")
}

func main() {
	app := fiber.New()
	var bad = godotenv.Load()
	if bad != nil {
		log.Fatal("Error loading .env file")
	}
	apihost := os.Getenv("HOST")
	dburl := os.Getenv("REDIS")
	con, con_err := redis.ParseURL(dburl)
	if con_err != nil {
		log.Fatalf("Error  while connecting  to database %s", con_err)
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
		duration, err := strconv.Atoi(c.Query("dur"))
		if err != nil {
			res_err := "Invalid Params Structure"
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Orginalurl: nil, Err: &res_err, Status: nil})
		}
		if duration > 3600 {
			res_err := "High Duration! not supported"
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Orginalurl: nil, Err: &res_err, Status: nil})
		}
		search, search_rerr := SearchUrl(uri)
		if search_rerr != nil {
			res_err := search_rerr.Error()
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Orginalurl: nil, Err: &res_err, Status: nil})
		}
		uuid, fullstr := GenrateUid(search)
		status := client.SetNX(ctx, uuid[:7], fullstr, time.Duration(duration)*time.Second).Val()
		c.Response().Header.Add("Cache-Control", "max-age="+fmt.Sprint(duration))
		shorted := apihost + "/" + uuid[:7]
		stat := !status
		return c.Status(fiber.StatusOK).JSON(Shortend{Duration: &duration, Url: &shorted, Status: &stat})
	})
	app.Get("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if check_id, check_error := CheckID(id, client); check_error != nil {
			return c.Status(fiber.StatusBadRequest).SendString(check_error.Error())
		} else {
			return c.Redirect(check_id)
		}
	})
	app.Post("/api/bulk", func(c *fiber.Ctx) error {
		full_req_body := c.Body()
		obj := string(full_req_body)
		var data Arr
		unmarshal_err := json.Unmarshal([]byte(obj), &data)
		if unmarshal_err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Unsupported Data Sturcter")
		}
		if parse_err := c.BodyParser(&data); parse_err != nil {
			res_err := "Invalid Data Structure"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{Invalid_body: &res_err, Too_many: nil, No_urls: nil})
		}
		if len(data.Urls) == 0 {
			res_err := "no urls!"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{Invalid_body: nil, Too_many: nil, No_urls: &res_err})
		}
		if len(data.Urls) > 50 {
			res_err := "maximum urls to shorten is 50!"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{Invalid_body: nil, Too_many: &res_err, No_urls: nil})
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
						res_err := "duration too high"
						ca <- Shortend{Orginalurl: &origin, Duration: &duration, Url: nil, Err: &res_err, Status: nil}
						return
					}
					if duration == 0 {
						duration += 60
					}
					search, search_err := SearchUrl(origin)
					if search_err != nil {
						res_err := search_err.Error()
						ca <- Shortend{Orginalurl: &origin, Duration: nil, Url: nil, Err: &res_err, Status: nil}
						return
					}
					u, fullstr := GenrateUid(search)
					done := apihost + "/" + u[:7]
					status := client.SetNX(ctx, u[:7], fullstr, time.Duration(duration)*time.Second).Val()
					stat := !status
					ca <- Shortend{Orginalurl: &origin, Duration: &duration, Url: &done, Err: nil, Status: &stat}
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
		port = "8443"
	}
	host := "0.0.0.0:" + port
	if listen_err := app.Listen(host); listen_err != nil {
		log.Fatalf("error while trying to listen -Error: %s", listen_err.Error())
	}
}
