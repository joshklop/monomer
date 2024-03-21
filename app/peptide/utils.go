package peptide

import (
	"log"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/samber/lo"
)

type Request interface {
	Marshal() ([]byte, error)
}

type Response interface {
	Unmarshal(resp []byte) error
}

type QueryableApp interface {
	Query(req abci.RequestQuery) (res abci.ResponseQuery)
}

// MustGetResponseWithHeight is a helper function that sends a query to a chain App and unmarshals the response into the
// provided response type.
// It panics if the query fails or if the response cannot be unmarshaled.
func MustGetResponseWithHeight[T Response](resp T, app QueryableApp, req Request, url string, height int64) T {
	queryResp := app.Query(abci.RequestQuery{Data: lo.Must(req.Marshal()), Path: url, Height: height})
	// panics if abci query response cannot be marshalled to type TResp
	lo.Must0(resp.Unmarshal(queryResp.Value))
	if queryResp.Code != 0 {
		log.Panicf("failed query with url: %s, response: %s", url, lo.Must(queryResp.MarshalJSON()))
	}
	return resp
}
