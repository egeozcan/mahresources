package constants

const MaxResultsPerPage = 50
const JSON = "application/json"
const HTML = "text/html"
const UrlEncodedForm = "application/x-www-form-urlencoded"
const MultiPartForm = "multipart/form-data"

// TimeFormat is used on the parsing of HTTP dates.
// This should never change unless the browsers start doing something crazy.
var TimeFormat = "2006-01-02T03:04"

var DbTypePosgres string = "POSTGRES"
var DbTypeSqlite string = "SQLITE"

var MaxThumbWidth float64 = 600
var MaxThumbHeight float64 = 600

var ThumbFileSuffix = ".thumbnail.jpg"
