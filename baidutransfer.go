package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	uid      = 1075874930
	cookie   = ""
	savepath = "/book"
)

const (
	post_fmt          = "http://yun.baidu.com/share/transfer?shareid=%s&from=%s&bdstoken=%s&channel=chunlei&clienttype=0&web=1&app_id=250528" //suguliang
	user_agent_iphone = "Mozilla/5.0 (iPhone; CPU iPhone OS 6_1_3 like Mac OS X) AppleWebKit/536.26 (KHTML, like Gecko) Version/6.0 Mobile/10B329 Safari/8536.25"
	user_agent_chrome = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/46.0.2490.86 Safari/537.36"
	list_fmt          = "http://yun.baidu.com/share/homerecord?uk=%d&page=%d&pagelength=60"

	//err
	ErrnoExist = 12
)

type Items struct {
	Errno int64  `json:"errno"`
	List  []item `json:"list"`
}

type item struct {
	ShareId         int64    `json:"shareId"`
	FsIds           []string `json:"fsIds"`
	Channel         int64    `json:"channel"`
	ChannelInfo     []int64  `json:"channelInfo"`
	Status          int64    `json:"status"`
	ExpiredType     int64    `json:"expiredType"`
	Passwd          string   `json:"passwd"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Ctime           int64    `json:"ctime"`
	AppId           int64    `json:"appId"`
	Public          int64    `json:"public"`
	PublicChannel   int64    `json:"publicChannel"`
	TplId           int64    `json:"tplId"`
	Shorturl        string   `json:"shorturl"`
	Tag             int64    `json:"tag"`
	Shareinfo       string   `json:"shareinfo"`
	Bitmap          int64    `json:"bitmap"`
	Port            int64    `json:"port"`
	TypicalCategory int64    `json:"typicalCategory"`
	TypicalPath     string   `json:"typicalPath"`
}

//input uid, cookie, savepath
func main() {
	flag.IntVar(&uid, "uid", 1075874930, "user id, get from web browser")
	flag.StringVar(&cookie, "cookie", "", "cookie, get from web browser, start with BDUSS=")
	flag.StringVar(&savepath, "savepath", "/book", "save path, should create in browser")
	flag.Parse()
	if len(cookie) == 0 {
		fmt.Println("cookie not set, get from web browser")
		return
	}
Loop:
	for i := 1; ; i++ {
		url := fmt.Sprintf(list_fmt, uid, i)
		body, err := HttpRequest("GET", url, map[string]string{
			"User-Agent":      user_agent_chrome,
			"Cookie":          cookie,
			"Accept-Encoding": "gzip, deflate",
		}, nil)
		checkError(err)
		var items Items
		err = json.Unmarshal(body, &items)
		checkError(err)
		n := len(items.List)
		for j := 0; j < n; j++ {
			if items.List[j].Ctime == 0 {
				break Loop
			}
			t := time.Unix(items.List[j].Ctime, 0)
			y, m, d := t.Date()
			h := t.Hour()
			min := t.Minute()
			when := fmt.Sprintf("%d-%02d-%02d %02d:%02d", y, m, d, h, min)
			fmt.Printf("%s\t%s\t%s\r\n", items.List[j].Shorturl, when, items.List[j].TypicalPath)

			url_str := fmt.Sprintf("http://yun.baidu.com/s/%s", items.List[j].Shorturl)
			if err := baiduTransfer(url_str, items.List[j].TypicalPath, savepath); err != nil {
				break Loop
			}
			fmt.Println("======================================================================")
		}
		if n == 0 {
			break
		}
		time.Sleep(time.Millisecond * 500)
	}
}

func baiduTransfer(url_str, file, path string) error {
	time.Sleep(time.Second * 1)
	body, err := HttpRequest("GET", url_str, map[string]string{
		"Host":            "pan.baidu.com",
		"User-Agent":      user_agent_iphone,
		"Cookie":          cookie,
		"Connection":      "keep-alive",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Accept-Encoding": "gzip, deflate",
		"Accept-Language": "zh-CN,zh;q=0.8",
	}, nil)
	checkError(err)

	shareid, uk, token := GetInfo(string(body))
	url_post := fmt.Sprintf(post_fmt, shareid, uk, token)
	time.Sleep(time.Second * 1)
	filelist := fmt.Sprintf("[\"%s\"]", file)
	body2, err := HttpRequest("POST", url_post, map[string]string{
		"User-Agent":       user_agent_chrome,
		"Cookie":           cookie,
		"Referer":          url_str,
		"Origin":           "http://yun.baidu.com",
		"X-Requested-With": "XMLHttpRequest",
		"Accept":           "*/*",
		"Accept-Language":  "zh-CN,zh;q=0.8",
		"Connection":       "keep-alive",
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
	}, map[string]string{
		"filelist": filelist,
		"path":     path,
	})
	checkError(err)
	fmt.Println(string(body2))
	resp := struct {
		Errno   int64       `json:"errno"`
		Task_id interface{} `json:"task_id"`
		Info    interface{} `json:"info"`
	}{}
	err = json.Unmarshal(body2, &resp)
	checkError(err)
	if resp.Errno == ErrnoExist { //stop when file exits
		return errors.New("need stop")
	}
	return nil
}

var cookieJar = &myCookieJar{}

const MAX_BODY_SIZE = 8 * 1024 * 1024 //8M
func HttpRequest(method, url_str string, head_map, form_map interface{}) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	url_obj, err := url.Parse(url_str)
	if err != nil {
		return buffer.Bytes(), errors.New("Url format error")
	}
	client := &http.Client{Jar: cookieJar}
	var dataBody io.Reader
	if form_map != nil {
		switch v := form_map.(type) {
		case map[string]string:
			form_value := url.Values{}
			for key, value := range v {
				form_value.Set(key, value)
			}
			dataBody = strings.NewReader(form_value.Encode())
		case string:
			dataBody = strings.NewReader(v)
		case []byte:
			dataBody = bytes.NewReader(v)
		default:
			return buffer.Bytes(), errors.New("The form arg is not a map")
		}
	}
	request, err := http.NewRequest(method, url_obj.String(), dataBody)
	if err != nil {
		return buffer.Bytes(), errors.New("HTTP GET failed")
	}

	if head_map != nil {
		switch v := head_map.(type) {
		case map[string]string:
			for key, value := range v {
				request.Header.Add(key, value)
			}
		default:
			return buffer.Bytes(), errors.New("The head arg is not a map")
		}
	}
	for _, c := range cookieJar.cookies {
		request.AddCookie(c)
	}

	response, err := client.Do(request)

	if err != nil {
		return buffer.Bytes(), errors.New("HTTP GET failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return buffer.Bytes(), errors.New(fmt.Sprintf("HTTP GET failed, status code %d", response.StatusCode))
	}

	buf := make([]byte, 4096)
	var rd io.Reader
	switch response.Header.Get("Content-Encoding") {
	case "gzip":
		gzipRd, err := gzip.NewReader(response.Body)
		if err != nil {
			return buffer.Bytes(), errors.New("HTTP body read error")
		}
		defer gzipRd.Close()
		rd = gzipRd
	case "deflate":
		flateRd := flate.NewReader(response.Body)
		defer flateRd.Close()
		rd = flateRd
	default:
		rd = bufio.NewReader(response.Body)
	}

	for {
		n, err := rd.Read(buf)
		if err != nil && err != io.EOF {
			return buffer.Bytes(), errors.New("HTTP body read error")
		}
		if n == 0 {
			break
		}
		buffer.Write(buf[:n])
		if buffer.Len() > MAX_BODY_SIZE {
			return buffer.Bytes(), errors.New("HTTP body too larg, partial readed")
		}
	}

	return buffer.Bytes(), nil
}

func multipartForm(form_map map[string]string, boundary string) []byte {
	buf := bytes.Buffer{}
	crlf := []byte{0x0d, 0x0a}
	disFmt := `Content-Disposition: form-data; name="%s"`
	for key, value := range form_map {
		buf.WriteString("--")
		buf.WriteString(boundary)
		buf.Write(crlf)
		keyStr := fmt.Sprintf(disFmt, key)
		buf.WriteString(keyStr)
		buf.Write(crlf)
		buf.Write(crlf)
		buf.WriteString(value)
		buf.Write(crlf)
	}

	buf.WriteString("--")
	buf.WriteString(boundary)
	buf.WriteString("--") //end
	buf.Write(crlf)
	return buf.Bytes()
}

type myCookieJar struct {
	cookies []*http.Cookie
}

func (c *myCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if c.cookies == nil {
		c.cookies = make([]*http.Cookie, 0)
	}

	for _, it := range cookies {
		c.cookies = append(c.cookies, it)
	}
}

func (c *myCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return c.cookies
}

func GetInfo(body string) (shareid, uk, token string) {
	re_shareid := regexp.MustCompile(`yunData.SHARE_ID\s*=\s*"([0-9].*?)"`)
	array := re_shareid.FindStringSubmatch(body)
	if len(array) >= 2 {
		shareid = array[1]
	}

	re_uk := regexp.MustCompile(`yunData.MYUK\s*=\s*"([0-9].*?)"`)
	array = re_uk.FindStringSubmatch(body)
	if len(array) >= 2 {
		uk = array[1]
	}
	re_token := regexp.MustCompile(`yunData.MYBDSTOKEN\s*=\s*"([0-9a-z].*?)"`)
	array = re_token.FindStringSubmatch(body)
	if len(array) >= 2 {
		token = array[1]
	}

	//another method
	if len(shareid) == 0 || len(uk) == 0 || len(token) == 0 {
		re_shareid := regexp.MustCompile("FileUtils.shareid=\"([0-9].*?)\"")
		array := re_shareid.FindStringSubmatch(body)
		if len(array) >= 2 {
			shareid = array[1]
		}

		re_uk := regexp.MustCompile("FileUtils.uk=\"([0-9].*?)\"")
		array = re_uk.FindStringSubmatch(body)
		if len(array) >= 2 {
			uk = array[1]
		}
		re_token := regexp.MustCompile("FileUtils.bdstoken=\"([0-9a-z].*?)\"")
		array = re_token.FindStringSubmatch(body)
		if len(array) >= 2 {
			token = array[1]
		}
	}
	return shareid, uk, token
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}
