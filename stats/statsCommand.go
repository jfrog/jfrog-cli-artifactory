package stats

const displayLimit = 5

type Stats struct {
	Format       string
	AccessToken  string
	ServerId     string
	DisplayLimit int
}

type CommandRunner interface {
	Run() error
}

func NewStatsCommand() *Stats {
	return &Stats{DisplayLimit: displayLimit}
}

func (s *Stats) SetFormat(format string) *Stats {
	s.Format = format
	return s
}

func (s *Stats) SetAccessToken(token string) *Stats {
	s.AccessToken = token
	return s
}

func (s *Stats) SetServerId(id string) *Stats {
	s.ServerId = id
	return s
}

func (ss *Stats) Run() error {
	var cmd CommandRunner
	cmd = ss.NewArtifactoryStatsCommand()
	return cmd.Run()
}

func (ss *Stats) NewArtifactoryStatsCommand() *ArtifactoryStats {
	newStatsCommand := NewArtifactoryStatsCommand().
		SetServerId(ss.ServerId).
		SetAccessToken(ss.AccessToken).
		SetFormat(ss.Format).
		SetDisplayLimit(ss.DisplayLimit)
	return newStatsCommand
}
