package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

const webport = "80"

func main() {
	// connnect to postgres database
	db := initDB()
	db.Ping()

	// create session

	// create channel

	// create waitgroup

	// set up application configuration

	// set up mail

	// listen to web connection
}

func initDB() *sql.DB {
	conn := connectToDB()
	if conn == nil{
		log.Panicln("can't connect to database")
	}

	return conn
}

func connectToDB() *sql.DB {
	counts := 0
	dsn := os.Getenv("DSN")

	for {
		connection, err := openDB(dsn)
		if err != nil{
			log.Println("postgres database not yet ready yet ...")
		}else{
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

func openDB(dsn string) (*sql.DB, error){
	db, err := sql.Open("pgx", dsn)
	if err != nil{
		log.Println("Error connecting to database")
		return nil, err
	}

	err = db.Ping()
	if err != nil{
		return nil, err
	}

	return db, nil
}