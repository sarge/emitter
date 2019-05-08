package broker

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/emitter-io/emitter/internal/security"
)

type keygenForm struct {
	Key      string
	Channel  string
	TTL      int64
	Sub      bool
	Pub      bool
	Store    bool
	Load     bool
	Presence bool
	Extend   bool
	Response string
}

// perform type checking
func (f *keygenForm) parse(req *http.Request) bool {

	var ok bool
	var parsedOK = true

	canParseToBool := func(source string) (bool, bool) {

		value := req.FormValue(source)

		if value == "" {
			return false, true
		} else if value == "on" {
			return true, true
		} else if value == "off" {
			return false, true
		}

		return false, false
	}

	canParseToInt := func(source string) (int64, bool) {

		value := req.FormValue(source)

		if value == "" {
			return 0, true
		}

		b, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
		return b, true
	}

	if f.Sub, ok = canParseToBool("sub"); !ok {
		parsedOK = false
	}

	if f.Pub, ok = canParseToBool("pub"); !ok {
		parsedOK = false
	}

	if f.Store, ok = canParseToBool("store"); !ok {
		parsedOK = false
	}

	if f.Load, ok = canParseToBool("load"); !ok {
		parsedOK = false
	}

	if f.Presence, ok = canParseToBool("presence"); !ok {
		parsedOK = false
	}

	if f.Extend, ok = canParseToBool("extend"); !ok {
		parsedOK = false
	}

	if f.TTL, ok = canParseToInt("ttl"); !ok {
		parsedOK = false
	}

	f.Key = req.FormValue("key")
	f.Channel = req.FormValue("channel")

	return parsedOK
}

// validate required fields
func (f *keygenForm) isValid() bool {

	if f.Key == "" {
		f.Response = "Missing SecretKey"
		return false
	}

	if f.Channel == "" {
		f.Response = "Missing Channel"
		return false
	}

	return true
}

func (f *keygenForm) expires() time.Time {
	if f.TTL == 0 {
		return time.Unix(0, 0)
	}

	return time.Now().Add(time.Duration(f.TTL) * time.Second).UTC()
}

func (f *keygenForm) access() uint8 {
	required := security.AllowNone

	if f.Sub {
		required |= security.AllowRead
	}
	if f.Pub {
		required |= security.AllowWrite
	}
	if f.Load {
		required |= security.AllowLoad
	}
	if f.Store {
		required |= security.AllowStore
	}
	if f.Presence {
		required |= security.AllowPresence
	}
	if f.Extend {
		required |= security.AllowExtend
	}

	return required
}

func handleKeyGen(s *Service) http.HandlerFunc {

	var fs http.FileSystem = assets

	f, err := fs.Open("keygen.html")
	if err != nil {
		panic(err)
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}
	t, err := template.New("keygen").
		Funcs(template.FuncMap{
			"isChecked": func(checked bool) string {
				if checked {
					return "checked=\"true\""
				}
				return ""
			}}).
		Parse(string(b))
	if err != nil {
		panic(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		f := keygenForm{Sub: true}
		switch r.Method {
		case "GET":
			// do nothing
		case "POST":

			ok := f.parse(r)

			if ok {
				if f.isValid() {
					key, err := s.generateKey(f.Key, f.Channel, f.access(), f.expires())
					if err != nil {
						f.Response = err.Error()
					} else {
						f.Response = fmt.Sprintf("channel: %s\nsuccess: %s", f.Channel, key)
					}

				}
			} else {
				f.Response = "invalid arguments"
			}

		default:
			http.Error(w, http.ErrNotSupported.Error(), 405)
			return
		}

		err := t.Execute(w, f)
		if err != nil {
			log.Printf("template execute error: %s\n", err.Error())
			http.Error(w, "internal server error", 500)
		}
	}
}