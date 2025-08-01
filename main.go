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

	"github.com/google/uuid"
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

type WriteupInsertRequest struct {
	Question     string `json:"question" binding:"required"`
	Category     string `json:"category" binding:"required,oneof=TIU TWK TKP"`
	Answer       string `json:"answer" binding:"required"`
	WrongAnswer1 string `json:"wrong_answer_1" binding:"required"`
	WrongAnswer2 string `json:"wrong_answer_2" binding:"required"`
	WrongAnswer3 string `json:"wrong_answer_3" binding:"required"`
	Explanation  string `json:"explanation"`
	Type         string `json:"type" binding:"required"`
	IsPublic     string `json:"is_public"` // e.g. "1" or "0"
	AIgen        string `json:"ai_gen"`    // e.g. "1" atau "0"
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

	r.GET("/i", func(c *gin.Context) {
		c.File("./public/insert_question.html")
	})

	r.GET("/e", func(c *gin.Context) {
		c.File("./public/edit.html")
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

	r.GET("/e/q/:hash_id", func(c *gin.Context) {
		hashID := c.Param("hash_id")
		var w WriteupInsertRequest

		query := `SELECT question, category, answer, wrong_answer_1, wrong_answer_2, wrong_answer_3, explanation, type, is_public, ai_gen 
			  FROM skd_writeup WHERE hash_id = ?`
		err := db.QueryRow(query, hashID).Scan(&w.Question, &w.Category, &w.Answer, &w.WrongAnswer1, &w.WrongAnswer2, &w.WrongAnswer3,
			&w.Explanation, &w.Type, &w.IsPublic, &w.AIgen)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Writeup not found"})
			return
		}

		c.JSON(http.StatusOK, w)
	})

	r.DELETE("/q/:hash_id", func(c *gin.Context) {
		hashID := c.Param("hash_id")

		query := `DELETE FROM skd_writeup WHERE hash_id = ?`
		res, err := db.Exec(query, hashID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Writeup not found"})
			return
		}

		// ✅ This should be the ONLY output to client
		c.JSON(http.StatusOK, gin.H{"message": "Writeup deleted successfully"})
	})

	r.PUT("/e/q/:hash_id", func(c *gin.Context) {
		hashID := c.Param("hash_id")
		var req WriteupInsertRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		query := `
		UPDATE skd_writeup
		SET question = ?, category = ?, answer = ?, wrong_answer_1 = ?, wrong_answer_2 = ?, wrong_answer_3 = ?,
			explanation = ?, type = ?, is_public = ?, ai_gen = ?, updated_at = CURRENT_TIMESTAMP
		WHERE hash_id = ?
	`

		res, err := db.Exec(query, req.Question, req.Category, req.Answer, req.WrongAnswer1, req.WrongAnswer2, req.WrongAnswer3,
			req.Explanation, req.Type, req.IsPublic, req.AIgen, hashID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Writeup not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Writeup updated successfully"})
	})

	r.POST("/q", func(c *gin.Context) {
		var req WriteupInsertRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Generate hash_id misal UUID
		hashID := uuid.New().String()

		query := `
		INSERT INTO skd_writeup 
		(question, category, answer, wrong_answer_1, wrong_answer_2, wrong_answer_3, explanation, type, is_public, ai_gen, hash_id, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

		_, err := db.Exec(query,
			req.Question, req.Category, req.Answer, req.WrongAnswer1, req.WrongAnswer2, req.WrongAnswer3,
			req.Explanation, req.Type, req.IsPublic, req.AIgen, hashID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Writeup created successfully",
			"hash_id": hashID,
		})
	})

	r.Run(":3000")
}
