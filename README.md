# ugh - running a marathon

push data from strava to google sheets, where I've copied a training plan  

Expects the following environment variables to be present:
- `SHEET_ID`: the ID of the google spreadsheet to sync to
- `SHEETS_CREDENTIALS`: path to credentials file with permission to view/edit google spreadsheets
- `STRAVA_ACCESS_TOKEN`: an API token for Strava, obtained through some weird sequence of actions I can't recall atm