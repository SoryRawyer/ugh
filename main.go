package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
)

var authURL = "https://www.googleapis.com/auth/spreadsheets"

func getClient(config *oauth2.Config) *http.Client {
	// the file token.json stores access and refresh tokens.
	// it's created automatically when the authorization flow completes
	// for the first time
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to %v\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v\n", err)
	}

	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// request a token from ye olde web service
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link and type the authorization code: \n%v\n",
		authURL)
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v\n", err)
	}
	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// stravaActivity holds all relevant data for a particular strava activity
type stravaActivity struct {
	ID            int     `json:"id"`
	StartTime     string  `json:"start_date_local"`
	DistanceM     float64 `json:"distance"`
	Name          string  `json:"name"`
	DurationSec   int16   `json:"duration"`
	MovingTimeSec int16   `json:"moving_time"`

	startDate *time.Time
}

type stravaResponse []stravaActivity

func getStravaData(stravaToken string, client *http.Client) *stravaResponse {
	// uhhhh call the api and then get some data from there also
	req, err := http.NewRequest("GET", "https://www.strava.com/api/v3/activities", nil)
	if err != nil {
		log.Fatalf("couldn't create Strava GET request: %v", err)
	}

	authHeader := fmt.Sprintf("Bearer %s", stravaToken)
	req.Header.Add("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	activities := &stravaResponse{}
	err = json.NewDecoder(resp.Body).Decode(activities)
	if err != nil {
		log.Fatalf("Couldn't decode the strava response: %v", err)
	}
	return activities
}

func filterActivitiesForDay(date time.Time, activities []stravaActivity) []stravaActivity {
	var result []stravaActivity
	loc, _ := time.LoadLocation("America/New_York")
	for _, activity := range activities {
		activityDate, _ := time.ParseInLocation(time.RFC3339, activity.StartTime, loc)
		if activityDate.Day() == date.Day() &&
			activityDate.Month() == date.Month() &&
			activityDate.Year() == date.Year() {
			activity.startDate = &activityDate
			result = append(result, activity)
		}
	}
	return result
}

// TODO: figure out if this is actually the best way to create a 2d array in Go
func getSpreadsheetValueFromActivity(activity *stravaActivity, updateRange string) *sheets.ValueRange {
	// calculate the mileage from the distance in meters
	mileage := activity.DistanceM / 1600
	row := make([]interface{}, 2)
	formattedDate := strings.Split(activity.StartTime, "T")[0]
	row[0] = formattedDate
	row[1] = mileage
	rows := make([][]interface{}, 1)
	rows[0] = row
	return &sheets.ValueRange{
		MajorDimension: "ROWS",
		Range:          updateRange,
		Values:         rows,
	}
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	// some auth stuff that I apparently didn't get right the first time because
	// I didn't get write the first time (I got read-only, apparently)
	b, err := ioutil.ReadFile(os.Getenv("SHEETS_CREDENTIALS"))
	if err != nil {
		log.Fatalf("couldn't read the credential file, man: %v", err)
	}

	config, err := google.ConfigFromJSON(b, authURL)
	if err != nil {
		log.Fatalf("%v", err)
	}

	client := getClient(config)
	srv, err := sheets.New(client)
	if err != nil {
		log.Fatalf("Unable to get sheets client: %v\n", err)
	}

	// get activities from Strava and filter to get todays activity
	stravaToken := os.Getenv("STRAVA_ACCESS_TOKEN")
	httpClient := &http.Client{}
	activities := getStravaData(stravaToken, httpClient)

	// we should really be getting at most one activity per day for strava
	todaysActivities := filterActivitiesForDay(time.Now(), *activities)
	if len(todaysActivities) == 0 {
		log.Print("No activities today!")
		return
	}

	if len(todaysActivities) != 1 {
		log.Fatalf("Ambiguous number of activities: %v\n", len(todaysActivities))
	}

	// Read data from the raw "runs" sheet
	spreadsheetID := os.Getenv("SHEET_ID")
	readRange := "runs!A1:B"

	// Try to post that activity to the spreadsheet
	activity := todaysActivities[0]
	log.Debug(activity)
	value := getSpreadsheetValueFromActivity(&activity, readRange)
	appendCall := srv.Spreadsheets.Values.Append(spreadsheetID, readRange, value).ValueInputOption("USER_ENTERED")
	appendResp, err := appendCall.Do()
	log.Debug("append resp: %v\n", appendResp)
	if err != nil {
		log.Fatalf("Error appending to spreadsheet: %v", err)
	}
}
