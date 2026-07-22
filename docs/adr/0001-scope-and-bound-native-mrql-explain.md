# Scope and bound native MRQL explanation

MRQL explanation describes the Effective MRQL Query after authorization scope and execution policy are applied, so it must never reveal a broader unscoped query to a confined principal. Generated statements remain available through the existing explain capability, while database-native plans are admin-only, opt-in, and non-executing; `EXPLAIN ANALYZE` is excluded because executing arbitrary diagnostic queries would create avoidable load and timeout risk.
