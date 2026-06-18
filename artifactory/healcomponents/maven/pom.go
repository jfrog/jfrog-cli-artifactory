package maven

import "encoding/xml"

type pomDocument struct {
	XMLName   xml.Name `xml:"project"`
	Packaging string   `xml:"packaging"`
	Modules   struct {
		Module []string `xml:"module"`
	} `xml:"modules"`
}

func parsePom(data []byte) (*pomDocument, error) {
	var p pomDocument
	if err := xml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *pomDocument) isAggregator() bool {
	if p == nil {
		return false
	}
	if len(p.Modules.Module) == 0 {
		return false
	}
	// Maven requires packaging=pom for aggregators; treat missing packaging as jar (not aggregator).
	return p.Packaging == "" || p.Packaging == "pom"
}

func (p *pomDocument) modulePaths() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.Modules.Module...)
}
