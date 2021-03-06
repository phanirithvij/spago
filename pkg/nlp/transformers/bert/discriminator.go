// Copyright 2020 spaGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bert

import (
	"github.com/nlpodyssey/spago/pkg/mat/f64utils"
	"github.com/nlpodyssey/spago/pkg/ml/ag"
	"github.com/nlpodyssey/spago/pkg/ml/nn"
	"github.com/nlpodyssey/spago/pkg/ml/nn/activation"
	"github.com/nlpodyssey/spago/pkg/ml/nn/linear"
	"github.com/nlpodyssey/spago/pkg/ml/nn/stack"
	"math"
)

var (
	_ nn.Model     = &Discriminator{}
	_ nn.Processor = &DiscriminatorProcessor{}
)

// DiscriminatorConfig provides configuration settings for a BERT Discriminator.
type DiscriminatorConfig struct {
	InputSize        int
	HiddenSize       int
	HiddenActivation ag.OpName
	OutputActivation ag.OpName
}

// Discriminator is a BERT Discriminator model.
type Discriminator struct {
	*stack.Model
}

// NewDiscriminator returns a new BERT Discriminator model.
func NewDiscriminator(config DiscriminatorConfig) *Discriminator {
	return &Discriminator{
		Model: stack.New(
			linear.New(config.InputSize, config.HiddenSize),
			activation.New(config.HiddenActivation),
			linear.New(config.HiddenSize, 1),
			activation.New(config.OutputActivation),
		),
	}
}

// DiscriminatorProcessor implements a nn.Processor for a BERT Discriminator.
type DiscriminatorProcessor struct {
	*stack.Processor
}

// NewProc returns a new processor to execute the forward step.
func (m *Discriminator) NewProc(ctx nn.Context) nn.Processor {
	return &DiscriminatorProcessor{
		Processor: m.Model.NewProc(ctx).(*stack.Processor),
	}
}

// Discriminate returns 0 or 1 for each encoded element, where 1 means that
// the word is out of context.
func (p *DiscriminatorProcessor) Discriminate(encoded []ag.Node) []int {
	ys := make([]int, len(encoded))
	for i, x := range p.Processor.Forward(encoded...) {
		ys[i] = int(math.Round(float64(f64utils.Sign(x.ScalarValue())+1.0) / 2.0))
	}
	return ys
}
