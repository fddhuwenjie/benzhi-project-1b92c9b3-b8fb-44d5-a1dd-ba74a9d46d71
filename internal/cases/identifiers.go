package cases

import "fmt"

func incidentIdentifier(day string, seq int) string { return fmt.Sprintf("INC-%s-%03d", day, seq) }
