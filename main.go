package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"

	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const (
	templatePath = "./templates"
	layoutPath   = templatePath + "/layout.html"
	createPath   = templatePath + "/create.html"
	editPath     = templatePath + "/edit.html"
	publicPath   = "./public"

	dbPath = "./db.sqlite3"

	// テーブル作成
	createPostTableQuery = `CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		body TEXT,
		author TEXT,
		created_at INTEGER
	)`
	// ユーザーテーブル作成
	createUserTableQuery = `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		created_at INTEGER
	)`

	// ブログポストテーブルにデータを挿入するSQL文
	insertPostQuery     = `INSERT INTO posts (title, body, author, created_at) VALUES (?, ?, ?, ?)`
	selectPostByIdQuery = `SELECT * FROM posts WHERE id = ?`
	// ブログポストテーブルから全てのデータを取得するSQL文
	selectAllPostsQuery = `SELECT * FROM posts`
	// ブログポストテーブルのデータを削除するSQL文
	deletePostQuery = `DELETE FROM posts WHERE id = ?`
	// ブログポストテーブルのデータを更新するSQL文
	updatePostQuery = `UPDATE posts SET title = ?, body = ?, author = ?, created_at = ? WHERE id = ?`

	// ユーザーテーブルからユーザーを取得するSQL文
	selectUserByUsernameQuery = `SELECT * FROM users WHERE username = ?`
)

type Post struct {
	ID        int64  `db:"id"`
	Title     string `db:"title"`
	Body      string `db:"body"`
	Author    string `db:"author"`
	CreatedAt int64  `db:"created_at"`
}

type User struct {
	ID        int64  `db:"id"`
	Username  string `db:"username"`
	Password  string `db:"password"`
	CreatedAt int64  `db:"created_at"`
}

var (
	db *sqlx.DB

	funcDate = template.FuncMap{
		"date": func(t int64) string {
			return time.Unix(t, 0).Format("2006-01-02 15:04:05")
		},
	}

	// セッションストアの初期化
	store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))

	indexTemplate  = template.Must(template.New("layout.html").Funcs(funcDate).ParseFiles(layoutPath, templatePath+"/index.html"))
	createTemplate = template.Must(template.ParseFiles(layoutPath, createPath))
	postTemplate   = template.Must(template.ParseFiles(layoutPath, templatePath+"/post.html"))
	editTemplate   = template.Must(template.ParseFiles(layoutPath, editPath))
	loginTemplate  = template.Must(template.ParseFiles(layoutPath, templatePath+"/login.html"))
	logoutTemplate = template.Must(template.ParseFiles(layoutPath, templatePath+"/logout.html"))
)

func main() {
	loadEnv()
	db = dbConnect()
	defer db.Close()
	err := initDB()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/post/", postHandler)
	http.HandleFunc("/post/delete/", deletePostHandler)
	http.HandleFunc("/post/edit/", editPostHandler)
	http.HandleFunc("/post/new", createPostHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir(publicPath+"/css"))))
	fmt.Println("Server is running on port http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// .envファイルを読み込む
func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	posts, err := getAllPosts()
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return
	}
	fmt.Printf("%+v\n", posts)
	indexTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"PageTitle": "記事一覧",
		"Posts":     posts,
	})
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// GETリクエストの場合はテンプレートを表示
		createTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
			"PageTitle": "ブログポスト作成",
		})
	} else if r.Method == "POST" {
		// POSTリクエストの場合はブログポストを作成
		title := r.FormValue("title")
		body := r.FormValue("body")
		author := r.FormValue("author")
		createdAt := time.Now().Unix()
		// フォームに空の項目がある場合はエラーを返す
		if title == "" || body == "" || author == "" {
			log.Print("フォームに空の項目があります")
			createTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
				"Message": "フォームに空の項目があります",
			})
			return
		}
		id, err := insertPost(title, body, author, createdAt)
		if err != nil {
			log.Print(err)
			return
		}
		// 作成したブログポストを表示
		http.Redirect(w, r, "/post/"+strconv.FormatInt(id, 10), 301)
	}
}

// ブログポストを更新
func editPostHandler(w http.ResponseWriter, r *http.Request) {
	// URLのPathからIDを取得
	id := r.URL.Path[len("/post/edit/"):]
	//idをint型に変換
	idInt, err := strconv.Atoi(id)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return
	}
	if r.Method == "GET" {
		// GETリクエストの場合はテンプレートを表示
		post, err := getPostByID(idInt)
		if err != nil {
			log.Print(err)
			// InternalServerErrorを返す
			return
		}
		editTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
			"PageTitle": "ブログポスト編集",
			"ID":        post.ID,
			"Title":     post.Title,
			"Body":      post.Body,
			"Author":    post.Author,
		})
	} else if r.Method == "POST" {
		// POSTリクエストの場合はブログポストを更新
		title := r.FormValue("title")
		body := r.FormValue("body")
		author := r.FormValue("author")
		createdAt := time.Now().Unix()
		// フォームに空の項目がある場合はエラーを返す
		if title == "" || body == "" || author == "" {
			log.Print("フォームに空の項目があります")
			editTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
				"Message": "フォームに空の項目があります",
			})
			return
		}
		err := updatePostByID(idInt, title, body, author, createdAt)
		if err != nil {
			log.Print(err)
			return
		}
		http.Redirect(w, r, "/post/"+strconv.Itoa(idInt), http.StatusFound)
	}
}

// ブログポストを削除
func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	// URLからIDを取得
	idStr := r.URL.Path[len("/post/delete/"):]
	// idをint型に変換
	idInt, err := strconv.Atoi(idStr)
	// ブログポストを削除
	err = deletePostByID(idInt)
	if err != nil {
		log.Print(err)
		return
	}
	// トップページにリダイレクト
	http.Redirect(w, r, "/", http.StatusFound)
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	// URLからIDを取得
	idStr := r.URL.Path[len("/post/"):]
	// idをint型に変換
	idInt, err := strconv.Atoi(idStr)
	// ブログポストを取得
	post, err := getPostByID(idInt)
	if err != nil {
		log.Print(err)
		return
	}
	// ブログポストを表示
	postTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"Title":     post.Title,
		"ID":        post.ID,
		"PageTitle": post.Title,
		"Body":      post.Body,
		"CreatedAt": time.Unix(post.CreatedAt, 0).Format("2006-01-02 15:04:05"),
		"Author":    post.Author,
	})
}

// ブログポストを作成
func insertPost(title string, body string, author string, createdAt int64) (int64, error) {
	// ブログポストテーブルにデータを挿入　last_insert_rowid()で最後に挿入したデータのIDを取得
	result, err := db.Exec(insertPostQuery, title, body, author, createdAt)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return 0, err
	}
	return id, nil
}

// 全てのブログポストを取得
func getAllPosts() ([]Post, error) {
	// ブログポストを全て取得
	var posts []Post
	err := db.Select(&posts, selectAllPostsQuery)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return posts, err
	}
	return posts, nil
}

// ブログポストをidから取得
func getPostByID(id int) (Post, error) {
	var post Post
	// idからブログポストを取得
	err := db.Get(&post, selectPostByIdQuery, id)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return post, err
	}
	return post, nil
}

func dbConnect() *sqlx.DB {
	// SQLite3のデータベースに接続
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func initDB() error {
	// ブログポストテーブルを作成
	_, err := db.Exec(createPostTableQuery)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return err
	}
	// ユーザーテーブルを作成
	_, err = db.Exec(createUserTableQuery)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return err
	}
	return nil
}

// ブログポストを削除
func deletePostByID(id int) error {
	_, err := db.Exec(deletePostQuery, id)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return err
	}
	return nil
}

// ブログポストを更新
func updatePostByID(id int, title string, body string, author string, createdAt int64) error {
	// ブログポストを更新
	_, err := db.Exec(updatePostQuery, title, body, author, createdAt, id)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return err
	}
	return nil
}

// ユーザー認証関数
func authenticateUser(username string, password string) (bool, error) {
	// ユーザーを取得
	var user User
	err := db.Get(&user, selectUserByUsernameQuery, username)
	if err != nil {
		log.Print(err)
		// InternalServerErrorを返す
		return false, err
	}
	// パスワードを比較
	// passwordはユーザーが入力したパスワード
	// user.Passwordはデータベースに保存されているハッシュ化されたパスワード
	match := checkPasswordHash(password, user.Password)
	return match, nil
}

// パスワードハッシュのチェック
func checkPasswordHash(password string, hash string) bool {
	// パスワードが一致するかチェック
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		log.Print(err)
		return false
	}
	return true
}

// ログインページのハンドラ
func loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// GETリクエストの場合はテンプレートを表示
		loginTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
			"PageTitle": "ログイン",
			"Message":   "",
		})
	case "POST":
		// POSTリクエストの場合はログイン処理を実行
		username := r.FormValue("username")
		password := r.FormValue("password")
		// ユーザー認証
		authenticated, err := authenticateUser(username, password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if authenticated {
			session, _ := store.Get(r, "session") // セッションを取得
			session.Values["authenticated"] = true
			session.Save(r, w) // セッションを保存
			// トップページにリダイレクト
			http.Redirect(w, r, "/", http.StatusFound)
		} else {
			// 認証に失敗した場合はログインページにリダイレクト
			http.Redirect(w, r, "/login", http.StatusFound)
		}
	}
}

// ログアウトページのハンドラ
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// GETリクエストの場合はテンプレートを表示
		logoutTemplate.ExecuteTemplate(w, "layout.html", map[string]interface{}{
			"PageTitle": "ログアウト",
		})
	case "POST":
		// POSTリクエストの場合はログアウト処理を実行
		session, _ := store.Get(r, "session") // セッションを取得
		session.Values["authenticated"] = false
		session.Save(r, w) // セッションを保存
		// トップページにリダイレクト
		http.Redirect(w, r, "/", http.StatusFound)
	}
}
