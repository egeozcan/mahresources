package constants

const MaxResultsPerPage = 50
const JSON = "application/json"
const HTML = "text/html"
const UrlEncodedForm = "application/x-www-form-urlencoded"
const MultiPartForm = "multipart/form-data"

// TimeFormat is used on the parsing of HTTP dates.
// This should never change unless the browsers start doing something crazy.
var TimeFormat = "2006-01-02T15:04"

var DbTypePosgres string = "POSTGRES"
var DbTypeSqlite string = "SQLITE"

var MaxThumbWidth float64 = 600
var MaxThumbHeight float64 = 600

var ThumbFileSuffix = ".thumbnail.jpg"

// ResourceTableIndexBuildLockKey is the PostgreSQL advisory lock a server holds
// while it builds indexes CONCURRENTLY on the resources, notes and groups
// tables: the FTS prefix indexes (on all three), the resource list indexes and
// the [adjacent] sibling navigation indexes ("MRFTSSMP", the value the FTS
// prefix build used alone before). Two such builds on one table from different
// servers deadlock rather than wait for each other, so they share this lock.
const ResourceTableIndexBuildLockKey = int64(0x4D52465453534D50)

// SimilarityDuplicateMaxDistance is the largest perceptual distance that counts
// as a near-certain duplicate rather than merely similar. The resource page's
// Similar Resources panel (models.Resource.SimilarityTier) and the [similar]
// shortcode both read it, so the two tiers cannot drift apart.
const SimilarityDuplicateMaxDistance = 2
