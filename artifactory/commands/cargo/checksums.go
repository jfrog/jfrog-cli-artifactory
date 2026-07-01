package cargo

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// aqlExecutor is the minimal seam over the Artifactory services manager used for
// checksum enrichment. *ArtifactoryServicesManagerImp satisfies it via Aql(string).
type aqlExecutor interface {
	Aql(aql string) (io.ReadCloser, error)
}

// aqlChecksumPageSize bounds how many crate names go in one AQL query.
const aqlChecksumPageSize = 100

// aqlResult is the subset of the AQL response we consume.
type aqlResult struct {
	Results []struct {
		Name       string `json:"name"`
		ActualSha1 string `json:"actual_sha1"`
		Sha256     string `json:"sha256"`
		ActualMd5  string `json:"actual_md5"`
	} `json:"results"`
}

// missingChecksumNames returns the crate filenames (dep.Id) of dependencies that
// are missing all checksum fields, de-duplicated, across all modules.
func missingChecksumNames(bi *entities.BuildInfo) []string {
	seen := map[string]bool{}
	var names []string
	if bi == nil {
		return names
	}
	for _, m := range bi.Modules {
		for _, d := range m.Dependencies {
			if d.Sha1 == "" && d.Sha256 == "" && d.Md5 == "" && d.Id != "" && !seen[d.Id] {
				seen[d.Id] = true
				names = append(names, d.Id)
			}
		}
	}
	return names
}

// chunk splits names into pages of at most size.
func chunk(names []string, size int) [][]string {
	if size <= 0 {
		size = 1
	}
	var pages [][]string
	for i := 0; i < len(names); i += size {
		end := i + size
		if end > len(names) {
			end = len(names)
		}
		pages = append(pages, names[i:end])
	}
	return pages
}

// buildChecksumAql builds an AQL query for one repo and a batch of crate filenames.
func buildChecksumAql(repo string, names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf(`{"name":%q}`, n)
	}
	return fmt.Sprintf(
		`items.find({"repo":%q,"$or":[%s]}).include("name","actual_sha1","sha256","actual_md5")`,
		repo, strings.Join(quoted, ","),
	)
}

// parseChecksumResults parses an AQL response body into name -> Checksum.
func parseChecksumResults(r io.Reader) (map[string]entities.Checksum, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var res aqlResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse aql response: %w", err)
	}
	out := map[string]entities.Checksum{}
	for _, it := range res.Results {
		out[it.Name] = entities.Checksum{Sha1: it.ActualSha1, Sha256: it.Sha256, Md5: it.ActualMd5}
	}
	return out, nil
}

// queryChecksums runs the batched, paginated AQL queries for one repo and merges results.
func queryChecksums(exec aqlExecutor, repo string, names []string) (map[string]entities.Checksum, error) {
	merged := map[string]entities.Checksum{}
	for _, page := range chunk(names, aqlChecksumPageSize) {
		aql := buildChecksumAql(repo, page)
		body, err := exec.Aql(aql)
		if err != nil {
			return merged, err
		}
		parsed, perr := parseChecksumResults(body)
		closeErr := body.Close()
		if perr != nil {
			return merged, perr
		}
		if closeErr != nil {
			log.Debug("cargo: aql body close: " + closeErr.Error())
		}
		for k, v := range parsed {
			merged[k] = v
		}
	}
	return merged, nil
}

// applyChecksums fills empty dependency checksums from the name->Checksum map.
// Returns the number of dependencies updated.
func applyChecksums(bi *entities.BuildInfo, byName map[string]entities.Checksum) int {
	filled := 0
	for mi := range bi.Modules {
		deps := bi.Modules[mi].Dependencies
		for di := range deps {
			d := &deps[di]
			if d.Sha1 != "" || d.Sha256 != "" || d.Md5 != "" {
				continue
			}
			if cs, ok := byName[d.Id]; ok && (cs.Sha1 != "" || cs.Sha256 != "" || cs.Md5 != "") {
				d.Sha1, d.Sha256, d.Md5 = cs.Sha1, cs.Sha256, cs.Md5
				filled++
			}
		}
	}
	return filled
}

// enrichMissingChecksums queries Artifactory for any dependency checksums the local
// cargo cache did not provide, and fills them in. It logs a reconciliation of how many
// were resolved locally vs from Artifactory. Missing repo or executor is a no-op.
func enrichMissingChecksums(bi *entities.BuildInfo, repo string, exec aqlExecutor) error {
	if bi == nil || repo == "" || exec == nil {
		return nil
	}
	missing := missingChecksumNames(bi)
	if len(missing) == 0 {
		log.Debug("cargo: all dependency checksums resolved from local cache; skipping AQL")
		return nil
	}
	log.Debug(fmt.Sprintf("cargo: %d dependencies missing checksums; querying Artifactory repo %q via AQL", len(missing), repo))
	byName, err := queryChecksums(exec, repo, missing)
	if err != nil {
		return err
	}
	filled := applyChecksums(bi, byName)
	stillMissing := len(missing) - filled
	log.Debug(fmt.Sprintf("cargo: checksum enrichment — %d filled from Artifactory, %d still missing", filled, stillMissing))
	if stillMissing > 0 {
		log.Warn(fmt.Sprintf("cargo: %d dependencies still missing checksums after AQL enrichment", stillMissing))
	}
	return nil
}
