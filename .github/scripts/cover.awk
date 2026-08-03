# cover.awk turns a Go coverage profile into a per-package Markdown table.
#
# Usage: awk -f .github/scripts/cover.awk coverage.out
#
# It is not named coverage.awk because .gitignore ignores coverage.* — the
# profiles a local `go test -cover` leaves behind.
#
# A profile written by `go test -coverprofile` starts with a "mode:" line and
# then carries one line per basic block:
#
#	<import path>/<file>.go:<line>.<col>,<line>.<col> <statements> <count>
#
# `go test ./...` runs one test binary per package, and `-coverprofile`
# concatenates what each of them wrote, so with `-coverpkg=./...` the same
# block appears once per test binary that could have reached it. The counts
# have to be summed before anything is totalled, or a block covered by two
# packages' tests is weighed twice. Doing that here is what makes the total
# printed below agree with `go tool cover -func`, which merges the same way.
#
# Written for POSIX awk: ubuntu-latest runs mawk, so no gawk-only builtins
# (asort, asorti, length(array)) and no sort(1) after the fact — the rows are
# ordered here so that the table reads the same on every run.

NR == 1 { next }  # "mode: set|count|atomic"

{
	statements[$1] = $2
	count[$1] += $3
}

END {
	for (block in statements) {
		# "path/to/file.go:12.34,56.7" -> "path/to/file.go" -> "path/to"
		pkg = block
		sub(/:[0-9].*$/, "", pkg)
		sub(/\/[^\/]*$/, "", pkg)

		if (!(pkg in total)) packages[++n] = pkg
		total[pkg] += statements[block]
		if (count[block] > 0) covered[pkg] += statements[block]
	}

	# Insertion sort: n is the number of packages in one module, so the cost
	# of the simplest thing that works is not worth measuring.
	for (i = 2; i <= n; i++) {
		pkg = packages[i]
		for (j = i - 1; j >= 1 && packages[j] > pkg; j--) packages[j + 1] = packages[j]
		packages[j + 1] = pkg
	}

	print "| Package | Covered | Statements | Coverage |"
	print "| --- | ---: | ---: | ---: |"
	for (i = 1; i <= n; i++) {
		pkg = packages[i]
		row(pkg, covered[pkg] + 0, total[pkg])
		allCovered += covered[pkg] + 0
		allTotal += total[pkg]
	}
	row("**Total**", allCovered, allTotal)
}

# row prints one line of the table. A package holding no statements at all —
# one that is only declarations, or only test files — is reported as such
# rather than as a division by zero.
function row(name, hits, stmts) {
	if (stmts == 0) {
		printf "| %s | %d | %d | n/a |\n", name, hits, stmts
		return
	}
	printf "| %s | %d | %d | %.1f%% |\n", name, hits, stmts, 100 * hits / stmts
}
