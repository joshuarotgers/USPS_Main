package buildinfo

var (
    Version = "dev"
    Commit  = ""
    BuiltAt = ""
)

func Info() map[string]string {
    return map[string]string{
        "version": Version,
        "commit":  Commit,
        "builtAt": BuiltAt,
    }
}

