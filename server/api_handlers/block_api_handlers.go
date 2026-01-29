package api_handlers

import (
	"encoding/json"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
)

func GetBlocksHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		noteID := uint(http_utils.GetIntQueryParameter(request, "noteId", 0))
		if noteID == 0 {
			http_utils.HandleError(nil, writer, request, http.StatusBadRequest)
			return
		}

		blocks, err := ctx.GetBlocksForNote(noteID)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(blocks)
	}
}

func GetBlockHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		block, err := ctx.GetBlock(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func CreateBlockHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.NoteBlockEditor

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.CreateBlock(&editor)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func UpdateBlockContentHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))

		var body struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.UpdateBlockContent(id, body.Content)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func UpdateBlockStateHandler(ctx interfaces.BlockStateWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))

		var body struct {
			State json.RawMessage `json:"state"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.UpdateBlockState(id, body.State)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func DeleteBlockHandler(ctx interfaces.BlockDeleter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))

		if err := ctx.DeleteBlock(id); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

func ReorderBlocksHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body query_models.NoteBlockReorderEditor
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.ReorderBlocks(body.NoteID, body.Positions); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}
