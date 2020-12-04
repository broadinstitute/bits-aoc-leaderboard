package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	firebase "firebase.google.com/go"
	"google.golang.org/api/iterator"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

//User object
type User struct {
	ID    int    `firestore:"id"`
	Name  string `firestore:"name"`
	Stars int    `firestore:"stars"`
}

func main() {
	http.HandleFunc("/", rootHandler) // Update this line of code

	fmt.Printf("Starting server at port 9000\n")
	if err := http.ListenAndServe(":9000", nil); err != nil {
		log.Fatal(err)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}
	var users []User
	const freq float64 = 15
	diff := time.Now().Sub(getLastUpdateTime())
	if diff.Minutes() > freq {
		log.Println("updating")
		users = refreshLeaderData()
		createNewRecord(users)
		//fmt.Fprintf(w, users)
	} else {
		log.Println("using cache")
		users = readLastRecord()
		//fmt.Fprintf(w, string(users))
	}

	js, err := json.Marshal(users)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func refreshLeaderData() []User {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://adventofcode.com/2020/leaderboard/private/view/712082.json", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("cookie", getSessionCookie())
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	users := []User{}
	result := make(map[string]interface{})

	json.NewDecoder(resp.Body).Decode(&result)

	members := result["members"].(map[string]interface{})
	for _, v := range members {
		userdata := v.(map[string]interface{})
		id, _ := strconv.Atoi(userdata["id"].(string))
		name := userdata["name"].(string)
		stars := int(userdata["stars"].(float64))

		users = append(users, User{ID: id, Name: name, Stars: stars})
	}

	return users
}

func getSessionCookie() string {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to setup client: %v", err)
	}

	// Build the request.
	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
		Name: "projects/982959808527/secrets/aoc-session-cookie/versions/1",
	}

	// Call the API.
	result, err := client.AccessSecretVersion(ctx, accessRequest)
	if err != nil {
		log.Fatalf("failed to access secret version: %v", err)
	}

	return string(result.Payload.Data)
}

func newClient() (context.Context, *firestore.Client) {
	ctx := context.Background()
	conf := &firebase.Config{ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT")}
	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		log.Fatalln(err)
	}

	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	return ctx, client
}

func createNewRecord(users []User) {
	ctx, client := newClient()
	defer client.Close()

	doc, _, err := client.Collection("Leaderboard").Add(ctx, map[string]interface{}{"timestamp": time.Now()})
	if err != nil {
		log.Fatal(err)
	}

	for _, user := range users {
		_, _, err = doc.Collection("Stats").Add(ctx, user)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getLastUpdateTime() time.Time {
	ctx, client := newClient()
	defer client.Close()

	leaderboard := client.Collection("Leaderboard")
	query := leaderboard.OrderBy("timestamp", firestore.Desc).Limit(1)
	iter := query.Documents(ctx)
	doc, err := iter.Next()

	if err != nil {
		log.Fatal(err)
	}
	lastUpdated := doc.Data()
	return lastUpdated["timestamp"].(time.Time)
}

func readLastRecord() []User {
	ctx, client := newClient()
	defer client.Close()

	users := []User{}

	leaderboard := client.Collection("Leaderboard")
	query := leaderboard.OrderBy("timestamp", firestore.Desc).Limit(1)
	iter := query.Documents(ctx)
	doc, err := iter.Next()

	if err != nil {
		log.Fatal(err)
	}

	stats := doc.Ref.Collection("Stats")
	iter = stats.Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		var u User

		err = doc.DataTo(&u)
		if err != nil {
			fmt.Println(err)
		}
		users = append(users, u)
	}
	return users
}
