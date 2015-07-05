package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Redefine the structs sent by the blockexplorer here. As they are
// sent through the modules module and the api as an interface{}, they
// are only defined in the blockexplorer module of Sia, which should
// not be imported from an external program
type (
	responseData struct {
		ResponseType string
	}
)

// hashPage handles the delegation of the pages depending on the hash type
func (es *ExploreServer) hashPageHandler(w http.ResponseWriter, r *http.Request) {
	var d []byte
	_, err := fmt.Sscanf(r.FormValue("h"), "%x", &d)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	itemJSON, err := es.apiGetHash(d)

	// Now decode the json and figure out which display function
	// to dispatch it to.
	var rd responseData
	err = json.Unmarshal(itemJSON, &rd)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	switch rd.ResponseType {
	case "Block":
		es.blockPage(w, itemJSON)
		return
	case "Transaction":
		es.txPage(w, itemJSON)
		return
	case "Output":
		es.outputPage(w, itemJSON, d[:crypto.HashSize])
		return
	case "Address":
		es.addressPage(w, itemJSON, d)
		return
	case "FileContract":
		es.contractPage(w, itemJSON, d)
	default:
		http.Error(w, "Siad returned: "+string(itemJSON), 500)
	}
}

func (es *ExploreServer) parseTransaction(hash crypto.Hash) ([]byte, error) {
	// Don't attempt to parse zero hashes
	if hash == *new(crypto.Hash) {
		return []byte{}, nil
	}
	// Decode into a responseData struct to figure out
	// what type of response it is, then switch on it
	itemJSON, err := es.apiGetHash(hash[:])
	var rd responseData
	err = json.Unmarshal(itemJSON, &rd)
	if err != nil {
		return nil, err
	}

	switch rd.ResponseType {
	case "Block":
		var b modules.BlockResponse
		err := json.Unmarshal(itemJSON, &b)
		if err != nil {
			return nil, err
		}

		// The block page requires additional
		// information contained in the block summary
		blockSummaries, err := es.apiGetBlockData(b.Height, b.Height+1)
		if err != nil {
			return nil, err
		}

		parsed, err := es.parseTemplate("block.template", blockRoot{
			Block:  b.Block,
			Height: b.Height,
			Target: blockSummaries[0].Target,
			Size:   blockSummaries[0].Size,
		})
		if err != nil {
			return nil, err
		}
		return parsed, nil
	case "Transaction":
		var t modules.TransactionResponse
		err := json.Unmarshal(itemJSON, &t)
		if err != nil {
			return nil, err
		}
		// Parse the main transaction template
		parsed, err := es.parseTemplate("transaction.template", t)
		if err != nil {
			return nil, err
		}
		return parsed, nil
	}
	return []byte{}, nil
}

// parseTransactions iterates over a list of transactions and puts
// each one into an html template, returning the concatinated list
func (es *ExploreServer) parseTransactions(hashes []crypto.Hash) ([]byte, error) {
	var page []byte

	// Request all the other things to be viewed
	for _, hash := range hashes {
		parsed, err := es.parseTransaction(hash)
		if err != nil {
			return nil, err
		}
		page = append(page, parsed...)
	}
	return page, nil
}

// blockPage formats information about a block for viewing
func (es *ExploreServer) blockPage(w http.ResponseWriter, blockJSON []byte) {
	var b modules.BlockResponse
	err := json.Unmarshal(blockJSON, &b)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	blockSummaries, err := es.apiGetBlockData(b.Height, b.Height+1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// blockRoot is defined in template.go
	page, err := es.parseTemplate("block.html", blockRoot{
		Block:  b.Block,
		Height: b.Height,
		Target: blockSummaries[0].Target,
		Size:   blockSummaries[0].Size,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(page)
}

// txPage formats information about a transaction in a format suitable
// for human viewing
func (es *ExploreServer) txPage(w http.ResponseWriter, txJSON []byte) {
	var t modules.TransactionResponse
	err := json.Unmarshal(txJSON, &t)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Parse the main transaction template
	page, err := es.parseTemplate("transaction.html", t)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(page)
}

// outputPage formats an outputTransactions struct for use in viewing
func (es *ExploreServer) outputPage(w http.ResponseWriter, outJSON []byte, oID []byte) {
	var ot modules.OutputResponse
	err := json.Unmarshal(outJSON, &ot)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var outputID crypto.Hash
	copy(outputID[:], oID[:])

	// Parse the main output template
	page, err := es.parseTemplate("output.html", outputRoot{
		OutputTx: ot.OutputTx,
		InputTx:  ot.InputTx,
		OutputID: outputID,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(page)
}

// addressPage formats the list of transactions that an address
// participated in for human consumption
func (es *ExploreServer) addressPage(w http.ResponseWriter, addrJSON []byte, address []byte) {
	var ad modules.AddrResponse
	err := json.Unmarshal(addrJSON, &ad)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Parse header template
	page, err := es.parseTemplate("address.html", addressRoot{
		Txns: ad.Txns,
		Addr: address,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(page)
}

func (es *ExploreServer) contractPage(w http.ResponseWriter, fcJSON []byte, fcid []byte) {
	var fi modules.FcResponse
	err := json.Unmarshal(fcJSON, &fi)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var FcID types.FileContractID
	copy(FcID[:], fcid[:])

	// Parse header template
	page, err := es.parseTemplate("contract.html", filecontractRoot{
		Contract:  fi.Contract,
		Revisions: fi.Revisions,
		Proof:     fi.Proof,
		FcID:      FcID,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(page)
}
