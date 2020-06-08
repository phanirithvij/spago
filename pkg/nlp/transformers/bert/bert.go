// Copyright 2020 spaGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bert

import (
	"encoding/json"
	"github.com/nlpodyssey/spago/pkg/ml/ag"
	"github.com/nlpodyssey/spago/pkg/ml/nn"
	"github.com/nlpodyssey/spago/pkg/ml/nn/linear"
	"github.com/nlpodyssey/spago/pkg/nlp/vocabulary"
	"log"
	"os"
)

var (
	_ nn.Model     = &Model{}
	_ nn.Processor = &Processor{}
)

type Config struct {
	HiddenAct             string `json:"hidden_act"`
	HiddenSize            int    `json:"hidden_size"`
	IntermediateSize      int    `json:"intermediate_size"`
	MaxPositionEmbeddings int    `json:"max_position_embeddings"`
	NumAttentionHeads     int    `json:"num_attention_heads"`
	NumHiddenLayers       int    `json:"num_hidden_layers"`
	TypeVocabSize         int    `json:"type_vocab_size"`
	VocabSize             int    `json:"vocab_size"`
}

func LoadConfig(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer configFile.Close()
	err = json.NewDecoder(configFile).Decode(&config)
	if err != nil {
		log.Fatal(err)
	}
	return config
}

type Model struct {
	Vocabulary      *vocabulary.Vocabulary
	Embeddings      *Embeddings
	Encoder         *Encoder
	Predictor       *Predictor
	Discriminator   *Discriminator // used by "ELECTRA" training method
	Pooler          *Pooler
	SeqRelationship *linear.Model
}

// NewDefaultBERT returns a new model based on the original BERT architecture.
func NewDefaultBERT(config Config, embeddingsStoragePath string) *Model {
	return &Model{
		Vocabulary: nil,
		Embeddings: NewEmbeddings(EmbeddingsConfig{
			Size:                config.HiddenSize,
			OutputSize:          config.HiddenSize,
			MaxPositions:        config.MaxPositionEmbeddings,
			TokenTypes:          config.TypeVocabSize,
			WordsMapFilename:    embeddingsStoragePath,
			WordsMapReadOnly:    false,
			DeletePreEmbeddings: false,
		}),
		Encoder: NewBertEncoder(EncoderConfig{
			Size:                   config.HiddenSize,
			NumOfAttentionHeads:    config.NumAttentionHeads,
			IntermediateSize:       config.IntermediateSize,
			IntermediateActivation: ag.OpGeLU,
			NumOfLayers:            config.NumHiddenLayers,
		}),
		Predictor: NewPredictor(PredictorConfig{
			InputSize:        config.HiddenSize,
			HiddenSize:       config.HiddenSize,
			OutputSize:       config.VocabSize,
			HiddenActivation: ag.OpGeLU,
			OutputActivation: ag.OpIdentity, // implicit Softmax (trained with CrossEntropyLoss)
		}),
		Discriminator: NewDiscriminator(DiscriminatorConfig{
			InputSize:        config.HiddenSize,
			HiddenSize:       config.HiddenSize,
			HiddenActivation: ag.OpGeLU,
			OutputActivation: ag.OpIdentity, // implicit Sigmoid (trained with BCEWithLogitsLoss)
		}),
		Pooler: NewPooler(PoolerConfig{
			InputSize:  config.HiddenSize,
			OutputSize: config.HiddenSize,
		}),
		SeqRelationship: linear.New(config.HiddenSize, 2),
	}
}

type Processor struct {
	nn.BaseProcessor
	Embeddings      *EmbeddingsProcessor
	Encoder         *EncoderProcessor
	Predictor       *PredictorProcessor
	Discriminator   *DiscriminatorProcessor
	Pooler          *PoolerProcessor
	SeqRelationship *linear.Processor
}

func (m *Model) NewProc(g *ag.Graph) nn.Processor {
	return &Processor{
		BaseProcessor: nn.BaseProcessor{
			Model:             m,
			Mode:              nn.Training,
			Graph:             g,
			FullSeqProcessing: true,
		},
		Embeddings:      m.Embeddings.NewProc(g).(*EmbeddingsProcessor),
		Encoder:         m.Encoder.NewProc(g).(*EncoderProcessor),
		Predictor:       m.Predictor.NewProc(g).(*PredictorProcessor),
		Discriminator:   m.Discriminator.NewProc(g).(*DiscriminatorProcessor),
		Pooler:          m.Pooler.NewProc(g).(*PoolerProcessor),
		SeqRelationship: m.SeqRelationship.NewProc(g).(*linear.Processor),
	}
}

func (p *Processor) SetMode(mode nn.ProcessingMode) {
	p.Mode = mode
	nn.SetProcessingMode(mode, p.Embeddings, p.Encoder, p.Predictor, p.Pooler, p.SeqRelationship)
}

func (p *Processor) Encode(tokens []string) []ag.Node {
	tokensEncoding := p.Embeddings.Encode(tokens)
	return p.Encoder.Forward(tokensEncoding...)
}

func (p *Processor) PredictMasked(transformed []ag.Node, masked []int) map[int]ag.Node {
	return p.Predictor.PredictMasked(transformed, masked)
}

func (p *Processor) Discriminate(encoded []ag.Node) []int {
	return p.Discriminator.Discriminate(encoded)
}

// Pool "pools" the model by simply taking the hidden state corresponding to the `[CLS]` token.
func (p *Processor) Pool(transformed []ag.Node) ag.Node {
	return p.Pooler.Forward(transformed[0])[0]
}

func (p *Processor) PredictSeqRelationship(pooled ag.Node) ag.Node {
	return p.SeqRelationship.Forward(pooled)[0]
}

func (p *Processor) Forward(_ ...ag.Node) []ag.Node {
	panic("bert: method not implemented")
}
