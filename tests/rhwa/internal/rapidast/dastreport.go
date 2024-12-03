package rapidast

// DASTReport struct that receives the results of the rapidast scan.
type DASTReport struct {
	ClusterName string
	Resources   []struct {
		Name      string
		Namespace string
		Results   []struct {
			Target         string
			Class          string
			Type           string
			MisconfSummary struct {
				Success    int
				Failures   int
				Exceptions int
			}
			Misconfigurations []struct {
				Type        string
				ID          string
				AVDID       string
				Description string
				Message     string
				Namespace   string
				Query       string
				Resolution  string
				Severity    string
				PrimaryURL  string
				References  []string
				Status      string
			}
		}
	}
}
