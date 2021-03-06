// Copyright 2020 spaGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stackedembeddings provides convenient types to stack multiple word embedding representations by concatenating them.
// The concatenation is then followed by a linear layer. The latter has the double utility of being able to project
// the concatenated embeddings in a smaller dimension, and to further train the final words representation.
package stackedembeddings

import (
	"fmt"
	"github.com/nlpodyssey/spago/pkg/ml/ag"
	"github.com/nlpodyssey/spago/pkg/ml/nn"
	"github.com/nlpodyssey/spago/pkg/ml/nn/linear"
	"log"
)

// WordsEncoderProcessor extends an nn.Processor providing the Encode method to
// transform a string sequence into an encoded representation.
type WordsEncoderProcessor interface {
	nn.Processor
	// Encode transforms a string sequence into an encoded representation.
	Encode([]string) []ag.Node
}

var (
	_ nn.Model     = &Model{}
	_ nn.Processor = &Processor{}
)

// Model implements a stacked embeddings model.
// TODO: optional use of the projection layer?
// TODO: include an optional layer normalization?
type Model struct {
	WordsEncoders   []nn.Model
	ProjectionLayer *linear.Model
}

// NewProc returns a new processor to execute the forward step.
func (m *Model) NewProc(ctx nn.Context) nn.Processor {
	processors := make([]WordsEncoderProcessor, len(m.WordsEncoders))
	for i, encoder := range m.WordsEncoders {
		proc, ok := encoder.NewProc(ctx).(WordsEncoderProcessor)
		if !ok {
			log.Fatal(fmt.Sprintf(
				"stackedembeddings: impossible to instantiate a `WordsEncoderProcessor` at index %d", i))
		}
		processors[i] = proc
	}
	return &Processor{
		BaseProcessor: nn.BaseProcessor{
			Model:             m,
			Mode:              ctx.Mode,
			Graph:             ctx.Graph,
			FullSeqProcessing: true,
		},
		encoders:        processors,
		projectionLayer: m.ProjectionLayer.NewProc(ctx).(*linear.Processor),
	}
}

// Processor implements the nn.Processor interface for a stack-embeddings Model.
type Processor struct {
	nn.BaseProcessor
	encoders        []WordsEncoderProcessor
	projectionLayer *linear.Processor
}

// Encode transforms a string sequence into an encoded representation.
func (p *Processor) Encode(words []string) []ag.Node {
	encodingsPerWord := make([][]ag.Node, len(words))
	for _, encoder := range p.encoders {
		for wordIndex, encoding := range encoder.Encode(words) {
			encodingsPerWord[wordIndex] = append(encodingsPerWord[wordIndex], encoding)
		}
	}
	intermediateEncoding := make([]ag.Node, len(words))
	for wordIndex, encoding := range encodingsPerWord {
		if len(encoding) == 1 { // optimization
			intermediateEncoding[wordIndex] = encoding[0]
		} else {
			intermediateEncoding[wordIndex] = p.Graph.Concat(encoding...)
		}
	}
	return p.projectionLayer.Forward(intermediateEncoding...)
}

// Forward is not implemented for stacked embedding model Processor (it always panics).
// You should use Encode instead.
func (p *Processor) Forward(_ ...ag.Node) []ag.Node {
	panic("stackedembeddings: Forward() not implemented. Use Encode() instead.")
}
