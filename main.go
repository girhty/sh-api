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
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func searchID(input string) (string, error) {
	regex := regexp.MustCompile(`https?:\/\/(?:www\.)?([-\d\w.]{2,256}[\d\w]{2,6}\b)*(\/[?\/\d\w\=+&#.-]*)*`)
	m := regex.FindStringSubmatch(input)
	if m != nil {
		return m[0], nil
	}
	return "", fmt.Errorf("Url Format Not Supported")
}

type resp struct {
	Uid  string `json:"ID"`
	Dura int    `json:"duration"`
	Url  string `json:"ShortURL"`
}
type Bulk struct {
	UrlToshorten string `json:"url"`
	Duration     int    `json:"duration"`
}
type Arr struct {
	Urls []Bulk `json:"data"`
}
type Shortend struct {
	Url        *string `json:"ShortURL,omitempty"`
	Duration   *int    `json:"duration,omitempty"`
	Orginalurl *string `json:"Origin,omitempty"`
	Status     *bool   `json:"KeyExists,omitempty"`
	Err        *string `json:"Error,omitempty"`
}
type Resp struct {
	Short []Shortend `json:"data"`
}
type BasicERR struct {
	InvalidBody *string `json:"Body,omitempty"`
	TooMuchUrls *string `json:"toMany,omitempty"`
	NoUrls      *string `json:"err,omitempty"`
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
			return "", fmt.Errorf("Key Not found")
		}
		decodedBytes, err := base64.StdEncoding.DecodeString(ch)
		if err != nil {
			return "", fmt.Errorf("Error While Getting Url")
		}
		return string(decodedBytes), nil
	}
	return "", fmt.Errorf("Id Format Is Not Supported")
}

func main() {
	app := fiber.New()
	var bad = godotenv.Load()
	if bad != nil {
		log.Fatal("Error loading .env file")
	}
	apihost := os.Getenv("YOUR.HOST")
	redisurl := os.Getenv("YOUR.REDIS")
	conn, noconn := redis.ParseURL(redisurl)
	if noconn != nil {
		log.Fatal("Error  while connecting  to database")
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
		AllowMethods: "GET,POST",
	}))
	client := redis.NewClient(conn)
	app.Use(limiter.New())
	app.Get("/api", func(c *fiber.Ctx) error {
		uri := c.Query("url")
		dur, err := strconv.Atoi(c.Query("dur"))
		if err != nil {
			err := "Invalid Params Structure"
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Orginalurl: nil, Err: &err, Status: nil})
		}
		if dur > 3600 {
			err := "High Duration ,Not supported"
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Orginalurl: nil, Err: &err, Status: nil})
		}
		ser, sererr := searchID(uri)
		if sererr != nil {
			err := sererr.Error()
			return c.Status(fiber.StatusBadRequest).JSON(Shortend{Url: nil, Duration: nil, Orginalurl: nil, Err: &err, Status: nil})
		}
		u, fullstr := GenrateUid(ser)
		status := client.SetNX(ctx, u[:7], fullstr, time.Duration(dur)*time.Second).Val()
		c.Response().Header.Add("Cache-Control", "max-age="+fmt.Sprint(dur))
		shorted := apihost + "/" + u[:7]
		stat := !status
		return c.Status(fiber.StatusOK).JSON(Shortend{Duration: &dur, Url: &shorted, Status: &stat})
	})
	app.Get("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		res, reserr := CheckID(id, client)
		if reserr != nil {
			return c.Status(fiber.StatusBadRequest).SendString(reserr.Error())
		}
		return c.Redirect(res)
	})
	app.Post("/bulk", func(c *fiber.Ctx) error {
		order := c.Body()
		obj := string(order)
		var data Arr
		Parseerr := json.Unmarshal([]byte(obj), &data)
		if Parseerr != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Unsupported Data Sturcter")
		}
		if err := c.BodyParser(&data); err != nil {
			err := "Invalid Data Structure"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{InvalidBody: &err, TooMuchUrls: nil, NoUrls: nil})
		}
		if len(data.Urls) == 0 {
			err := "No URLS where provided"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{InvalidBody: nil, TooMuchUrls: nil, NoUrls: &err})
		}
		if len(data.Urls) > 50 {
			err := "urls amount above 50 not allowed"
			return c.Status(fiber.StatusBadRequest).JSON(BasicERR{InvalidBody: nil, TooMuchUrls: &err, NoUrls: nil})
		}
		var res_obj []Shortend
		for _, ur := range data.Urls {
			origin := ur.UrlToshorten
			duration := ur.Duration
			if duration > 3600 {
				err := "Duration Too high"
				res_obj = append(res_obj, Shortend{Orginalurl: &origin, Duration: &duration, Url: nil, Err: &err, Status: nil})
				continue
			}
			if duration == 0 {
				duration += 60
			}
			ser, searcherr := searchID(origin)
			if searcherr != nil {
				err := searcherr.Error()
				res_obj = append(res_obj, Shortend{Orginalurl: &origin, Duration: nil, Url: nil, Err: &err, Status: nil})
				continue
			}
			u, fullstr := GenrateUid(ser)
			shotrendone := apihost + "/" + u[:7]
			status := client.SetNX(ctx, u[:7], fullstr, time.Duration(duration)*time.Second).Val()
			stat := !status
			res_obj = append(res_obj, Shortend{Orginalurl: &origin, Duration: &duration, Url: &shotrendone, Err: nil, Status: &stat})
		}
		return c.Status(fiber.StatusCreated).JSON(Resp{Short: res_obj})
	})
	app.Listen("0.0.0.0:3000")
}
