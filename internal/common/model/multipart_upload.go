package model

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

const maxMultipartMetadataFieldBytes int64 = 1 << 20

// HandleMultipartFileStream streams a multipart file part without staging it on disk.
func HandleMultipartFileStream(r *http.Request, fileKey string, fileNameKey string, handleFile func(fileName string, file io.Reader) error) error {
	reader, err := r.MultipartReader()
	if err != nil {
		return &ParsingError{Err: err}
	}

	handler := multipartFileStreamHandler{
		fileKey:     fileKey,
		fileNameKey: fileNameKey,
		handleFile:  handleFile,
	}
	return handler.handleParts(reader)
}

type multipartFileStreamHandler struct {
	fileKey      string
	fileNameKey  string
	fileName     string
	bufferedFile *bufferedMultipartFile
	completed    bool
	handleFile   func(fileName string, file io.Reader) error
}

func (h *multipartFileStreamHandler) handleParts(reader *multipart.Reader) error {
	for {
		part, err := reader.NextPart()
		if err != nil {
			return h.handleNextPartError(err)
		}
		if err := h.handlePart(part); err != nil {
			return err
		}
		if h.completed {
			return nil
		}
	}
}

func (h *multipartFileStreamHandler) handleNextPartError(err error) error {
	if !errors.Is(err, io.EOF) {
		return &ParsingError{Err: err}
	}
	if h.bufferedFile == nil {
		return &ParsingError{Param: h.fileKey, Err: errors.New("multipart file field is required")}
	}
	return h.handleBufferedMultipartFile()
}

func (h *multipartFileStreamHandler) handlePart(part *multipart.Part) error {
	switch part.FormName() {
	case h.fileNameKey:
		return h.handleFileNamePart(part)
	case h.fileKey:
		return h.handleFilePart(part)
	default:
		discardMultipartPart(part)
		return nil
	}
}

func (h *multipartFileStreamHandler) handleFileNamePart(part *multipart.Part) error {
	value, readErr := readMultipartMetadataField(part)
	closeErr := part.Close()
	if readErr != nil {
		return &ParsingError{Param: h.fileNameKey, Err: readErr}
	}
	if closeErr != nil {
		return &ParsingError{Param: h.fileNameKey, Err: closeErr}
	}
	if value != "" && h.fileName == "" {
		h.fileName = value
	}
	return nil
}

func (h *multipartFileStreamHandler) handleFilePart(part *multipart.Part) error {
	if h.bufferedFile != nil {
		discardMultipartPart(part)
		return nil
	}
	if h.fileName != "" {
		if err := handleStreamingMultipartFile(part, h.fileKey, h.fileName, h.handleFile); err != nil {
			return err
		}
		h.completed = true
		return nil
	}

	bufferedFile, err := bufferMultipartFilePart(part, h.fileKey)
	if err != nil {
		return err
	}
	h.bufferedFile = bufferedFile
	return nil
}

func (h *multipartFileStreamHandler) handleBufferedMultipartFile() error {
	resolvedFileName := strings.TrimSpace(h.fileName)
	if resolvedFileName == "" {
		resolvedFileName = h.bufferedFile.fileName
	}
	return h.handleFile(resolvedFileName, bytes.NewReader(h.bufferedFile.content))
}

type bufferedMultipartFile struct {
	fileName string
	content  []byte
}

func handleStreamingMultipartFile(part *multipart.Part, fileKey string, fileName string, handleFile func(fileName string, file io.Reader) error) error {
	fileReader := &maxBytesTrackingReader{reader: part}
	handleErr := handleFile(fileName, fileReader)
	closeErr := part.Close()
	if maxBytesErr := fileReader.MaxBytesError(); maxBytesErr != nil {
		return &ParsingError{Param: fileKey, Err: maxBytesErr}
	}
	if handleErr != nil {
		return handleErr
	}
	if closeErr != nil {
		return multipartCloseError(fileKey, closeErr)
	}
	return nil
}

func bufferMultipartFilePart(part *multipart.Part, fileKey string) (*bufferedMultipartFile, error) {
	fileReader := &maxBytesTrackingReader{reader: part}
	var buffer bytes.Buffer
	_, copyErr := io.Copy(&buffer, fileReader)
	closeErr := part.Close()
	if maxBytesErr := fileReader.MaxBytesError(); maxBytesErr != nil {
		return nil, &ParsingError{Param: fileKey, Err: maxBytesErr}
	}
	if copyErr != nil {
		return nil, &ParsingError{Param: fileKey, Err: copyErr}
	}
	if closeErr != nil {
		return nil, multipartCloseError(fileKey, closeErr)
	}
	return &bufferedMultipartFile{
		fileName: strings.TrimSpace(part.FileName()),
		content:  buffer.Bytes(),
	}, nil
}

type maxBytesTrackingReader struct {
	reader      io.Reader
	maxBytesErr *http.MaxBytesError
}

func (r *maxBytesTrackingReader) Read(target []byte) (int, error) {
	readCount, err := r.reader.Read(target)
	if maxBytesErr := maxBytesErrorFromError(err); maxBytesErr != nil {
		r.maxBytesErr = maxBytesErr
	}
	return readCount, err
}

func (r *maxBytesTrackingReader) MaxBytesError() *http.MaxBytesError {
	return r.maxBytesErr
}

func maxBytesErrorFromError(err error) *http.MaxBytesError {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return maxBytesErr
	}
	return nil
}

func multipartCloseError(fileKey string, err error) error {
	if maxBytesErr := maxBytesErrorFromError(err); maxBytesErr != nil {
		return &ParsingError{Param: fileKey, Err: maxBytesErr}
	}
	return &ParsingError{Param: fileKey, Err: err}
}

func readMultipartMetadataField(part *multipart.Part) (string, error) {
	content, err := io.ReadAll(io.LimitReader(part, maxMultipartMetadataFieldBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(content)) > maxMultipartMetadataFieldBytes {
		return "", errors.New("multipart metadata field is too large")
	}
	return strings.TrimSpace(string(content)), nil
}

func discardMultipartPart(part *multipart.Part) {
	_, _ = io.Copy(io.Discard, part)
	_ = part.Close()
}
