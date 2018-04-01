package main

import (
	"flag"
	"os"
	"log"
	"net/url"
	"time"
	"fmt"
	"strconv"
	"net/http"
	"io/ioutil"
	"golang.org/x/net/html"
	"strings"
	"gopkg.in/gomail.v2"
	"crypto/tls"
)

const (
	expireDate       = "25/06/2026"
	apptType         = "CSCA"
	applicantType    = "CSCA_WALK_IN"
	htmlElementTD    = "td"
	htmlElementInput = "input"
	htmlAttrClass    = "class"
	htmlAttrName     = "name"
	htmlAttrValue    = "value"
)

var classValueFilter = map[string]bool{
	"cal_PLAIN": true,
	"cal_PH":    true,
	"cal_AF":    true,
	"cal_NA":    true,
}

var requestURL = flag.String(
	"requestURL",
	"https://eappointment.ica.gov.sg/ibook/publicLogin.do?nav=N",
	"Request the citizen slot entry point",
)

var nric = flag.String(
	"nric",
	"",
	"The request user nric",
)

var offset = flag.Duration(
	"offset",
	8,
	"The server deploy zone time offset, default is UTC-8",
)

var interval = flag.Duration(
	"interval",
	time.Minute * 5,
	"Each check available interval",
)

var checkRange = flag.Int(
	"checkRange",
	12,
	"Check total month",
)

var fromMail = flag.String(
	"fromMail",
	"",
	"Send mail",
)

var mailPassword = flag.String(
	"mailPassword",
	"",
	"Send email password",
)

var smtp = flag.String(
	"smtp",
	"smtp.gmail.com",
	"Send mail",
)

var toMail = flag.String(
	"toMail",
	"",
	"Receive mail",
)

var debug = flag.Bool(
	"debug",
	false,
	"Debug model",
)

var monitorInterval = flag.Duration(
	"monitorInterval",
	time.Hour,
	"How long time send the monitor email",
)

func main() {

	flag.Parse()

	//sendMail(nextNMonthDateStr(0), *fromMail)

	validate()

	round := 0

	// monitor logic
	go func() {
		ticker := time.NewTicker(*monitorInterval)
		for range ticker.C {
			sendMail("internal check have run round " + strconv.Itoa(round), *fromMail)
		}

	}()

	for {

		round++

		log.Printf("start check the available %v", time.Now())

		for i := -1; i < *checkRange; i++ {

			content := GetContents(i)

			if isAvailable(content, nextNMonthDateStr(i+1)) {
				sendMail(nextNMonthDateStr(i), *toMail)
			}

			time.Sleep(time.Second)

		}

		time.Sleep(*interval)
	}


}

func validate() {
	if *requestURL == "" {
		log.Println("requestURL can not be empty")
		os.Exit(1)
	}
	if nric == nil || *nric == "" {
		log.Println("nric can not be empty")
		os.Exit(1)
	}
	if mailPassword == nil || *mailPassword == "" {
		log.Println("mailPassword can not be empty")
		os.Exit(1)
	}
}

func isAvailable(content string, nextMonthParam string) bool {

	strings.NewReader(content)

	z := html.NewTokenizer(strings.NewReader(content))

	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			// End of the document, we're done
			return false
		case tt == html.StartTagToken:
			t := z.Token()

			// <input type="hidden" name="calendar.startDate" value="01/04/2018">
			if htmlElementInput == t.Data && GetAttrVal(t, htmlAttrName) == "calendar.startDate" {
				if GetAttrVal(t, htmlAttrValue) != nextMonthParam {
					log.Printf("content info invalid, realVal=%s expectVal=%s", GetAttrVal(t, htmlAttrValue), nextMonthParam)
				}
				log.Printf("current query month=%s", GetAttrVal(t, htmlAttrValue))
			}

			if htmlElementTD != t.Data {
				continue
			}
			classVal := GetAttrVal(t, htmlAttrClass)

			_, ok := classValueFilter[classVal]
			if *debug {

				log.Println(GetAttrVal(t, htmlAttrClass))
			}

			if strings.HasPrefix(classVal, "cal_") && !ok {
				log.Printf("content %s", content)
				log.Printf("find the available td=%v", t)
				return true
			}

		}
	}

	return false
}

func GetAttrVal(t html.Token, key string) string {
	for _, a := range t.Attr {
		if a.Key == key {
			return a.Val
			break
		}
	}
	return ""
}

func GetContents(nextNMonth int) string {

	formData := generatePostForm(nextNMonth)

	log.Printf("form data = %v", formData)

	resp, err := http.PostForm(*requestURL, generatePostForm(nextNMonth))

	if err != nil {
		log.Printf("request error, %v", err)
		return ""
	}

	bytes, _ := ioutil.ReadAll(resp.Body)

	if *debug {
		log.Println("HTML:\n\n", string(bytes))
	}

	resp.Body.Close()

	return string(bytes)
}

/** -d apptDetails.apptType=CSCA \
-d apptDetails.identifier1=S9076665A \
-d apptDetails.identifier2=1 \
-d apptDetails.changeLimitFlg=N \
-d apptDetails.latestValidityStartDt=26/03/2018 \
-d apptDetails.earliestValidityEndDt=25/06/2026 \
-d apptDetails.applicantType=CSCA_WALK_IN \
-d calendar.previousCalDate=01/04/2018 \
-d calendar.calendarYearStr=2018 \
-d calendar.calendarMonthStr=4 \
-d calendar.startDate=01/05/2018 \
-d calendar.nextCalDate=01/06/2018 \
*/
func generatePostForm(nextNMonth int) url.Values {

	formData := url.Values{}
	formData.Set("apptDetails.apptType", apptType)
	formData.Set("apptDetails.identifier1", *nric)
	formData.Set("apptDetails.identifier2", "1")
	formData.Set("apptDetails.changeLimitFlg", "N")
	formData.Set("apptDetails.latestValidityStartDt", currentDate())
	formData.Set("apptDetails.earliestValidityEndDt", expireDate)
	formData.Set("apptDetails.applicantType", applicantType)
	formData.Set("calendar.previousCalDate", nextNMonthDateStr(nextNMonth-1))
	formData.Set("calendar.startDate", nextNMonthDateStr(nextNMonth))
	formData.Set("calendar.nextCalDate", nextNMonthDateStr(nextNMonth+1))
	formData.Set("calendar.calendarYearStr", strconv.Itoa(nextNMonthDate(nextNMonth).Year()))
	formData.Set("calendar.calendarMonthStr", strconv.Itoa(int(nextNMonthDate(nextNMonth).Month())-1))

	return formData
}

func currentDate() string {

	now := time.Now().Add(time.Hour * *offset)

	currentDate := fmt.Sprintf("%02d/%02d/%d", now.Day(), now.Month(), now.Year())

	return currentDate
}

func nextNMonthDate(n int) time.Time {
	now := time.Now().Add(time.Hour * *offset)

	nextMonth := now.AddDate(0, n, -1*(now.Day()-1))

	return nextMonth
}

func nextNMonthDateStr(n int) string {

	nextMonth := nextNMonthDate(n)

	nextMonthStr := fmt.Sprintf("%02d/%02d/%d", nextMonth.Day(), nextMonth.Month(), nextMonth.Year())

	return nextMonthStr
}

func sendMail(content string, to string) {

	m := gomail.NewMessage()
	m.SetHeader("From", *fromMail)
	m.SetHeader("To", strings.Split(to, ",")...)
	m.SetHeader("Subject", "Available Slot "+content)
	m.SetBody("text/html", "Please check the date <b>"+content+" </b>")

	//m.Attach("/home/Alex/lolcat.jpg")
	//m.SetAddressHeader("Cc", "dan@example.com", "Dan")

	// Send emails using d.
	d := gomail.NewDialer(*smtp, 587, *fromMail, *mailPassword)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	// Send the email to Bob, Cora and Dan.
	if err := d.DialAndSend(m); err != nil {
		panic(err)
	}
}
