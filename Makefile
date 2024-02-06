.PHONY: peptide
peptide:
	go build -o peptide ./cmd/peptide/

.PHONY: pepctl
pepctl:
	go build -o pepctl ./cmd/pepctl/

.PHONY: clean
clean:
	rm ./peptide ./pepctl
