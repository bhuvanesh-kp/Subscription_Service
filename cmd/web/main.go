package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"subscription_service/data"
	"sync"
	"syscall"
	"time"

	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gomodule/redigo/redis"
	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

const webport = "80"

func main() {
	// connnect to postgres database
	db := initDB()

	// create session
	session := initSession()

	// create logger
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime)

	// create channel

	// create waitgroup
	wg := &sync.WaitGroup{}

	// set up application configuration
	app := Config{
		Session:  session,
		DB:       db,
		InfoLog:  infoLog,
		ErrorLog: errorLog,
		Wait:     wg,
		Models:   data.New(db),
	}

	// set up mail\

	// listen for signals
	go app.listenForShutDown()

	// listen to web connection
	app.serve()
}

func (app *Config) serve() {
	srv := http.Server{
		Addr:    fmt.Sprintf(":%s", webport),
		Handler: app.routes(),
	}

	app.InfoLog.Println("Starting web server ...")
	err := srv.ListenAndServe()
	if err != nil {
		log.Panic(err)
	}
}

func initDB() *sql.DB {
	conn := connectToDB()
	if conn == nil {
		log.Panicln("can't connect to database")
	}

	return conn
}

func connectToDB() *sql.DB {
	counts := 0
	dsn := os.Getenv("DSN")
	fmt.Println(dsn)

	for {
		connection, err := openDB(dsn)
		if err != nil {
			log.Println("postgres database not yet ready yet ...")
		} else {
			log.Println("connected to database...")
			return connection
		}

		if counts > 10 {
			return nil
		}

		log.Println("Backing off for 1 second")
		time.Sleep(1 * time.Second)
		counts++
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Println("Error connecting to database")
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func initSession() *scs.SessionManager {
	// session set up
	session := scs.New()
	session.Store = redisstore.New(initRedis())
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = true

	return session
}

func initRedis() *redis.Pool {
	redisPool := &redis.Pool{
		MaxIdle: 10,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", os.Getenv("REDIS"))
		},
	}

	return redisPool
}

func (app *Config) listenForShutDown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	app.ShutDown()
	os.Exit(0)
}

func (app *Config) ShutDown() {
	app.InfoLog.Println("Running clean up tasks...")
	app.Wait.Wait()
	app.InfoLog.Println("closing channels and shutting down the application.")
}
