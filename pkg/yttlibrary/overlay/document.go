package overlay

import (
	"github.com/get-ytt/ytt/pkg/yamlmeta"
)

func (o OverlayOp) mergeDocument(
	leftDocSets []*yamlmeta.DocumentSet, newDoc *yamlmeta.Document) error {

	ann, err := NewDocumentMatchAnnotation(newDoc, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.IndexTuples(leftDocSets)
	if err != nil {
		return err
	}

	for _, leftIdx := range leftIdxs {
		replace, err := o.apply(leftDocSets[leftIdx[0]].Items[leftIdx[1]].Value, newDoc.Value)
		if err != nil {
			return err
		}
		if replace {
			leftDocSets[leftIdx[0]].Items[leftIdx[1]].Value = newDoc.Value
		}
	}

	return nil
}

func (o OverlayOp) removeDocument(
	leftDocSets []*yamlmeta.DocumentSet, newDoc *yamlmeta.Document) error {

	ann, err := NewDocumentMatchAnnotation(newDoc, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.IndexTuples(leftDocSets)
	if err != nil {
		return err
	}

	for _, leftIdx := range leftIdxs {
		leftDocSets[leftIdx[0]].Items[leftIdx[1]] = nil
	}

	return nil
}

func (o OverlayOp) replaceDocument(
	leftDocSets []*yamlmeta.DocumentSet, newDoc *yamlmeta.Document) error {

	ann, err := NewDocumentMatchAnnotation(newDoc, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.IndexTuples(leftDocSets)
	if err != nil {
		return err
	}

	for _, leftIdx := range leftIdxs {
		leftDocSets[leftIdx[0]].Items[leftIdx[1]] = newDoc.DeepCopy()
	}

	return nil
}

func (o OverlayOp) insertDocument(
	leftDocSets []*yamlmeta.DocumentSet, newDoc *yamlmeta.Document) error {

	ann, err := NewDocumentMatchAnnotation(newDoc, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.IndexTuples(leftDocSets)
	if err != nil {
		return err
	}

	insertAnn, err := NewInsertAnnotation(newDoc)
	if err != nil {
		return err
	}

	for i, leftDocSet := range leftDocSets {
		updatedDocs := []*yamlmeta.Document{}

		for j, leftItem := range leftDocSet.Items {
			matched := false
			for _, leftIdx := range leftIdxs {
				if leftIdx[0] == i && leftIdx[1] == j {
					matched = true
					if insertAnn.IsBefore() {
						updatedDocs = append(updatedDocs, newDoc.DeepCopy())
					}
					updatedDocs = append(updatedDocs, leftItem)
					if insertAnn.IsAfter() {
						updatedDocs = append(updatedDocs, newDoc.DeepCopy())
					}
					break
				}
			}
			if !matched {
				updatedDocs = append(updatedDocs, leftItem)
			}
		}

		leftDocSet.Items = updatedDocs
	}

	return nil
}

func (o OverlayOp) appendDocument(
	leftDocSets []*yamlmeta.DocumentSet, newDoc *yamlmeta.Document) error {

	// No need to traverse further
	leftDocSets[len(leftDocSets)-1].Items = append(leftDocSets[len(leftDocSets)-1].Items, newDoc.DeepCopy())
	return nil
}
