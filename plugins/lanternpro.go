package lanternpro

import (
	"net/http"
)

type LanternProPlugin struct {
}

func New() (*LanternProPlugin, error) {
	return nil, nil
}

func (f *LanternProPlugin) ServeHTTP(w http.ResponseWriter, req *http.Request) {
}
