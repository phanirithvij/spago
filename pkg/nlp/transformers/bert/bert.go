// Copyright 2020 spaGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bert

import (
	"encoding/json"
	"fmt"
	"github.com/nlpodyssey/spago/pkg/ml/ag"
	"github.com/nlpodyssey/spago/pkg/ml/nn"
	"github.com/nlpodyssey/spago/pkg/ml/nn/linear"
	"github.com/nlpodyssey/spago/pkg/nlp/vocabulary"
	"github.com/nlpodyssey/spago/pkg/utils"
	"log"
	"os"
	"path"
	"strconv"
)

const (
	// DefaultConfigurationFile is the default BERT JSON configuration filename.
	DefaultConfigurationFile = "config.json"
	// DefaultVocabularyFile is the default BERT model's vocabulary filename.
	DefaultVocabularyFile = "vocab.txt"
	// DefaultModelFile is the default BERT spaGO model filename.
	DefaultModelFile = "spago_model.bin"
	// DefaultEmbeddingsStorage is the default directory name for BERT model's embedding storage.
	DefaultEmbeddingsStorage = "embeddings_storage"
)

var (
	_ nn.Model     = &Model{}
	_ nn.Processor = &Processor{}
)

// Config provides configuration settings for a BERT Model.
type Config struct {
	HiddenAct             string            `json:"hidden_act"`
	HiddenSize            int               `json:"hidden_size"`
	IntermediateSize      int               `json:"intermediate_size"`
	MaxPositionEmbeddings int               `json:"max_position_embeddings"`
	NumAttentionHeads     int               `json:"num_attention_heads"`
	NumHiddenLayers       int               `json:"num_hidden_layers"`
	TypeVocabSize         int               `json:"type_vocab_size"`
	VocabSize             int               `json:"vocab_size"`
	ID2Label              map[string]string `json:"id2label"`
	ReadOnly              bool              `json:"read_only"`
}

// LoadConfig loads a BERT model Config from file.
func LoadConfig(file string) (Config, error) {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		return Config{}, err
	}
	defer configFile.Close()
	err = json.NewDecoder(configFile).Decode(&config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

// Model implements a BERT model.
type Model struct {
	Config          Config
	Vocabulary      *vocabulary.Vocabulary
	Embeddings      *Embeddings
	Encoder         *Encoder
	Predictor       *Predictor
	Discriminator   *Discriminator // used by "ELECTRA" training method
	Pooler          *Pooler
	SeqRelationship *linear.Model
	SpanClassifier  *SpanClassifier
	Classifier      *Classifier
}

// NewDefaultBERT returns a new model based on the original BERT architecture.
func NewDefaultBERT(config Config, embeddingsStoragePath string) *Model {
	return &Model{
		Config:     config,
		Vocabulary: nil,
		Embeddings: NewEmbeddings(EmbeddingsConfig{
			Size:                config.HiddenSize,
			OutputSize:          config.HiddenSize,
			MaxPositions:        config.MaxPositionEmbeddings,
			TokenTypes:          config.TypeVocabSize,
			WordsMapFilename:    embeddingsStoragePath,
			WordsMapReadOnly:    config.ReadOnly,
			DeletePreEmbeddings: false,
		}),
		Encoder: NewBertEncoder(EncoderConfig{
			Size:                   config.HiddenSize,
			NumOfAttentionHeads:    config.NumAttentionHeads,
			IntermediateSize:       config.IntermediateSize,
			IntermediateActivation: ag.OpGELU,
			NumOfLayers:            config.NumHiddenLayers,
		}),
		Predictor: NewPredictor(PredictorConfig{
			InputSize:        config.HiddenSize,
			HiddenSize:       config.HiddenSize,
			OutputSize:       config.VocabSize,
			HiddenActivation: ag.OpGELU,
			OutputActivation: ag.OpIdentity, // implicit Softmax (trained with CrossEntropyLoss)
		}),
		Discriminator: NewDiscriminator(DiscriminatorConfig{
			InputSize:        config.HiddenSize,
			HiddenSize:       config.HiddenSize,
			HiddenActivation: ag.OpGELU,
			OutputActivation: ag.OpIdentity, // implicit Sigmoid (trained with BCEWithLogitsLoss)
		}),
		Pooler: NewPooler(PoolerConfig{
			InputSize:  config.HiddenSize,
			OutputSize: config.HiddenSize,
		}),
		SeqRelationship: linear.New(config.HiddenSize, 2),
		SpanClassifier: NewSpanClassifier(SpanClassifierConfig{
			InputSize: config.HiddenSize,
		}),
		Classifier: NewTokenClassifier(ClassifierConfig{
			InputSize: config.HiddenSize,
			Labels: func(x map[string]string) []string {
				if len(x) == 0 {
					return []string{"LABEL_0", "LABEL_1"} // assume binary classification by default
				}
				y := make([]string, len(x))
				for k, v := range x {
					i, err := strconv.Atoi(k)
					if err != nil {
						log.Fatal(err)
					}
					y[i] = v
				}
				return y
			}(config.ID2Label),
		}),
	}
}

// LoadModel loads a BERT Model from file.
func LoadModel(modelPath string) (*Model, error) {
	configFilename := path.Join(modelPath, DefaultConfigurationFile)
	vocabFilename := path.Join(modelPath, DefaultVocabularyFile)
	embeddingsFilename := path.Join(modelPath, DefaultEmbeddingsStorage)
	modelFilename := path.Join(modelPath, DefaultModelFile)

	fmt.Printf("Start loading pre-trained model from \"%s\"\n", modelPath)
	fmt.Printf("[1/3] Loading configuration... ")
	config, err := LoadConfig(configFilename)
	if err != nil {
		return nil, err
	}
	fmt.Printf("ok\n")
	model := NewDefaultBERT(config, embeddingsFilename)

	fmt.Printf("[2/3] Loading vocabulary... ")
	vocab, err := vocabulary.NewFromFile(vocabFilename)
	if err != nil {
		return nil, err
	}
	fmt.Printf("ok\n")
	model.Vocabulary = vocab

	fmt.Printf("[3/3] Loading model weights... ")
	err = utils.DeserializeFromFile(modelFilename, nn.NewParamsSerializer(model))
	if err != nil {
		log.Fatal(fmt.Sprintf("bert: error during model deserialization (%s)", err.Error()))
	}
	fmt.Println("ok")

	return model, nil
}

// Processor implements the nn.Processor interface for a BERT Model.
type Processor struct {
	nn.BaseProcessor
	Embeddings      *EmbeddingsProcessor
	Encoder         *EncoderProcessor
	Predictor       *PredictorProcessor
	Discriminator   *DiscriminatorProcessor
	Pooler          *PoolerProcessor
	SeqRelationship *linear.Processor
	SpanClassifier  *SpanClassifierProcessor
	Classifier      *ClassifierProcessor
}

// NewProc returns a new processor to execute the forward step.
func (m *Model) NewProc(ctx nn.Context) nn.Processor {
	return &Processor{
		BaseProcessor: nn.BaseProcessor{
			Model:             m,
			Mode:              ctx.Mode,
			Graph:             ctx.Graph,
			FullSeqProcessing: true,
		},
		Embeddings:      m.Embeddings.NewProc(ctx).(*EmbeddingsProcessor),
		Encoder:         m.Encoder.NewProc(ctx).(*EncoderProcessor),
		Predictor:       m.Predictor.NewProc(ctx).(*PredictorProcessor),
		Discriminator:   m.Discriminator.NewProc(ctx).(*DiscriminatorProcessor),
		Pooler:          m.Pooler.NewProc(ctx).(*PoolerProcessor),
		SeqRelationship: m.SeqRelationship.NewProc(ctx).(*linear.Processor),
		SpanClassifier:  m.SpanClassifier.NewProc(ctx).(*SpanClassifierProcessor),
		Classifier:      m.Classifier.NewProc(ctx).(*ClassifierProcessor),
	}
}

// Encode transforms a string sequence into an encoded representation.
func (p *Processor) Encode(tokens []string) []ag.Node {
	tokensEncoding := p.Embeddings.Encode(tokens)
	return p.Encoder.Forward(tokensEncoding...)
}

// PredictMasked performs a masked prediction task. It returns the predictions
// for indices associated to the masked nodes.
func (p *Processor) PredictMasked(transformed []ag.Node, masked []int) map[int]ag.Node {
	return p.Predictor.PredictMasked(transformed, masked)
}

// Discriminate returns 0 or 1 for each encoded element, where 1 means that
// the word is out of context.
func (p *Processor) Discriminate(encoded []ag.Node) []int {
	return p.Discriminator.Discriminate(encoded)
}

// Pool "pools" the model by simply taking the hidden state corresponding to the `[CLS]` token.
func (p *Processor) Pool(transformed []ag.Node) ag.Node {
	return p.Pooler.Forward(transformed[0])[0]
}

// PredictSeqRelationship predicts if the second sentence in the pair is the
// subsequent sentence in the original document.
func (p *Processor) PredictSeqRelationship(pooled ag.Node) ag.Node {
	return p.SeqRelationship.Forward(pooled)[0]
}

// TokenClassification performs a classification for each element in the sequence.
func (p *Processor) TokenClassification(transformed []ag.Node) []ag.Node {
	return p.Classifier.Predict(transformed)
}

// SequenceClassification performs a single sentence-level classification,
// using the pooled CLS token.
func (p *Processor) SequenceClassification(transformed []ag.Node) ag.Node {
	return p.Classifier.Predict(p.Pooler.Forward(transformed[0]))[0]
}

// Forward is not implemented for BERT model Processor (it always panics).
func (p *Processor) Forward(_ ...ag.Node) []ag.Node {
	panic("bert: method not implemented")
}
