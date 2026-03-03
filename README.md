# sqlfmt

An opinionated SQL formatter.

## Installation

```
go install mibk.dev/sqlfmt@latest
```

## Usage

```
sqlfmt [-w] [-s] [path ...]
```

Without arguments, sqlfmt reads from stdin and writes the formatted output to stdout.
When given file or directory paths, it processes them and prints the result.
With directories, it recursively formats all `.sql` files.

Flags:

  - `-w` — write result to the source file instead of stdout
  - `-s` — simplify code

## Example

Before:

```sql
SELECT
p . id , DATE_FORMAT ( p.date ,'%Y-%m-%d' ) AS datestr
FROM purchase p
JOIN building b ON p.station_id=b.id
WHERE p.user_id IN (
select user_id From registry
Where registered <= curdate()
)
   ORDER BY p.date  ;
```

After:

```sql
SELECT
	p.id, DATE_FORMAT(p.date, '%Y-%m-%d') AS datestr
FROM purchase p
JOIN building b ON p.station_id = b.id
WHERE p.user_id IN (
	SELECT user_id
	FROM registry
	WHERE registered <= CURDATE()
)
ORDER BY p.date;
```

## License

[MIT](LICENSE)
