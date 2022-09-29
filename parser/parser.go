package parser

import (
	"context"
	"fmt"
	"io/ioutil"
	"regexp"

	"go.uber.org/zap"
	"inet.af/netaddr"
)

type Lease struct {
	Name    string
	Address netaddr.IP
}

func (l *Lease) GetName() string {
	return l.Name
}

func (l *Lease) GetAddress() netaddr.IP {
	return l.Address
}

func (l *Lease) String() string {
	return fmt.Sprintf("%v (%v)", l.Name, l.Address)
}

type parser struct {
	logger *zap.Logger
}

func NewParser(logger *zap.Logger) *parser {
	return &parser{logger: logger}
}

var leaseMatcher = regexp.MustCompile(`lease ([0-9a-f.:]+) {(?:[^}]|}[^\n]+")+client-hostname "([^"]+)";`)

func (p *parser) ParseFile(path string) ([]*Lease, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return []*Lease{}, err
	}
	return p.ParseData(string(content))
}

func (p *parser) ParseData(data string) ([]*Lease, error) {
	matches := leaseMatcher.FindAllStringSubmatch(data, -1)

	p.logger.Debug("matched file", zap.Int("count", len(matches)))

	leases := []*Lease{}
	for imatch := range matches {
		lease, err := parseMatch(matches[imatch])
		if err != nil {
			p.logger.Warn("unparsable lease match", zap.Error(err), zap.Strings("match", matches[imatch]))
			continue
		}
		leases = append(leases, lease)
	}

	p.logger.Debug("parsed leases", zap.Int("count", len(leases)), zap.Int("matches", len(matches)))
	return leases, nil
}

func (p *parser) ParseStreaming(ctx context.Context, path string) chan *Lease {
	ch := make(chan *Lease)

	go func() {
		parseCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		defer close(ch)

		content, err := ioutil.ReadFile(path)
		if err != nil {
			p.logger.Error("failed to open file", zap.Error(err), zap.String("path", path))
			return
		}

		count := 0
		searchIndex := 0
		for {
			if parseCtx.Err() != nil {
				p.logger.Error("stopped parsing", zap.Error(err))
				return
			}

			match := leaseMatcher.FindSubmatchIndex(content[searchIndex:])
			if match == nil {
				p.logger.Debug("no more matches in file", zap.String("path", path), zap.Int("count", count))
				break
			}

			matchedBytes := make([][]byte, len(match)/2)
			for i := 0; i < len(matchedBytes); i++ {
				matchedBytes[i] = content[searchIndex+match[i*2] : searchIndex+match[i*2+1]]
			}

			if lease, err := parseMatchBytes(matchedBytes); err == nil {
				ch <- lease
			} else {
				p.logger.Warn("failed to parse match", zap.Error(err), zap.ByteStrings("match", matchedBytes))
			}

			searchIndex += match[1]
			count += 1
		}
	}()
	return ch
}

type MatchHandler func(*Lease)

func (p *parser) ParseStreamingWithHandler(ctx context.Context, path string, handler MatchHandler) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := p.ParseStreaming(ctx, path)

	for {
		select {
		case lease, ok := <-ch:
			if !ok {
				ch = nil
				break
			}
			go handler(lease)
		case <-ctx.Done():
			ch = nil
		}

		if ch == nil {
			break
		}
	}
}

func parseMatchBytes(match [][]byte) (*Lease, error) {
	strs := make([]string, len(match))
	for i := range match {
		strs[i] = string(match[i])
	}
	return parseMatch(strs)
}

func parseMatch(match []string) (*Lease, error) {
	if len(match) != 3 {
		return nil, ErrInvalidGroupCount
	}

	address, err := netaddr.ParseIP(match[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	return &Lease{Name: match[2], Address: address}, nil
}

type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrInvalidGroupCount = Error("match has invalid length")
)
