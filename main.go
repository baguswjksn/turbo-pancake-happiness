package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/yuin/goldmark"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

type Question struct {
	ID          int
	Question    template.HTML
	Answer      template.HTML
	Category    string
	Type        string
	Explanation template.HTML
	Hash_ID     string
}

func parseMarkdown(md string) template.HTML {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(md)) // fallback to escaped text
	}
	return template.HTML(buf.String())
}

func main() {

	r := gin.Default()

	// Tambahkan fungsi ke FuncMap
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}

	r.GET("/favicon.svg", func(c *gin.Context) {
		c.File("./static/favicon.svg")
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.File("./static/favicon.svg") // fallback to svg
	})

	// Load template dengan FuncMap
	r.SetFuncMap(funcMap)
	r.LoadHTMLGlob("templates/*.html") // pastikan path sesuai

	r.Static("/public", "./public")

	r.GET("/", func(c *gin.Context) {
		c.File("./public/index.html")
	})

	r.GET("/rtl", func(c *gin.Context) {
		c.File("./public/rtl.html")
	})

	r.GET("/tl", func(c *gin.Context) {
		c.File("./public/timeline.html")
	})

	// Load .env file if present
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables only")
	}

	// Define and parse the mode flag
	mode := flag.String("mode", "dev", "Application mode: dev or prod")
	flag.Parse()

	// Choose DB path based on mode
	var dbPath string
	switch *mode {
	case "prod":
		dbPath = os.Getenv("PROD_DB_PATH")
		if dbPath == "" {
			dbPath = "./data/prod_database.db"
		}
	case "dev":
		dbPath = os.Getenv("DEV_DB_PATH")
		if dbPath == "" {
			dbPath = "./data/dev_database.db"
		}
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}

	fmt.Printf("Running in %s mode. Using DB: %s\n", *mode, dbPath)

	// Open the database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS skd_writeup (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		question TEXT NOT NULL,
		answer TEXT,
		explanation TEXT,
		category TEXT,
		type TEXT,
		is_public INTEGER DEFAULT 1
	)`
	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatal("Error creating table:", err)
	}

	r.GET("/d", func(c *gin.Context) {
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)
		pageSize := 25
		offset := (page - 1) * pageSize

		category := c.Query("category")
		qtype := c.Query("type")
		search := c.Query("search")

		where := "WHERE is_public = 1"
		var args []interface{}

		if category != "" {
			where += " AND category = ?"
			args = append(args, category)
		}
		if qtype != "" {
			where += " AND type = ?"
			args = append(args, qtype)
		}
		if search != "" {
			where += " AND question LIKE ?"
			args = append(args, "%"+search+"%")
		}

		query := fmt.Sprintf(`SELECT hash_id, question, category, type FROM skd_writeup %s LIMIT ? OFFSET ?`, where)
		args = append(args, pageSize, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var questions []Question
		for rows.Next() {
			var q Question
			if err := rows.Scan(&q.Hash_ID, &q.Question, &q.Category, &q.Type); err != nil {
				log.Println(err)
				continue
			}
			questions = append(questions, q)
		}

		c.HTML(http.StatusOK, "index.html", gin.H{
			"Questions": questions,
			"Page":      page,
			"PageSize":  pageSize,
			"Category":  category,
			"Type":      qtype,
			"Search":    search,
		})
	})

	r.GET("/f", func(c *gin.Context) {
		// Ambil query parameter dari URL
		category := c.Query("category")
		qType := c.Query("type")

		// Bangun query SQL dinamis
		query := "SELECT id, question, answer, explanation, category, type FROM skd_writeup WHERE is_public = 1"
		var args []interface{}
		if category != "" {
			query += " AND category = ?"
			args = append(args, category)
		}
		if qType != "" {
			query += " AND type = ?"
			args = append(args, qType)
		}

		query += " ORDER BY RANDOM() LIMIT 1"

		// Jalankan query
		row := db.QueryRow(query, args...)

		var rawExplanation, rawQuestion, rawAnswer string
		var q Question
		err := row.Scan(&q.ID, &rawQuestion, &rawAnswer, &rawExplanation, &q.Category, &q.Type)
		if err != nil {
			if err == sql.ErrNoRows {
				c.HTML(http.StatusOK, "flash.html", gin.H{
					"Question": nil,
				})
				return
			}
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		// Parse markdown for question, answer, and explanation
		q.Question = parseMarkdown(rawQuestion)
		q.Answer = parseMarkdown(rawAnswer)
		q.Explanation = parseMarkdown(rawExplanation)

		// Render template
		c.HTML(http.StatusOK, "flash.html", gin.H{
			"Question": q,
			"Query": gin.H{
				"Category": category,
				"Type":     qType,
			},
		})
	})

	r.GET("/q/:id", func(c *gin.Context) {
		hashID := c.Param("id")

		query := "SELECT hash_id, question, answer, explanation, category, type FROM skd_writeup WHERE hash_id = ? AND is_public = 1"
		row := db.QueryRow(query, hashID)

		var rawQuestion, rawAnswer, rawExplanation string
		var q Question

		err := row.Scan(&q.Hash_ID, &rawQuestion, &rawAnswer, &rawExplanation, &q.Category, &q.Type)
		if err != nil {
			if err == sql.ErrNoRows {
				c.HTML(http.StatusNotFound, "question.html", gin.H{
					"Question": nil,
				})
				return
			}
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		// Parse all markdown content
		q.Question = parseMarkdown(rawQuestion)
		q.Answer = parseMarkdown(rawAnswer)
		q.Explanation = parseMarkdown(rawExplanation)

		c.HTML(http.StatusOK, "question.html", gin.H{
			"Question": q,
		})
	})

	r.Run(":3000")
}
