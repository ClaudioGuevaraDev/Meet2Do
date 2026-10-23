package transcriber

import "github.com/ClaudioGuevaraDev/Meet2Do/internal/download"

// ProgressFunc recibe los bytes descargados y el total (0 o -1 si se desconoce).
type ProgressFunc = download.ProgressFunc
