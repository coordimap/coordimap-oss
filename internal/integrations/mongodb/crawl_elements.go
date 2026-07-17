package mongodb

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/coordimap/agent/pkg/domain/database"
	"github.com/coordimap/agent/pkg/domain/mongodb"
	"github.com/gertd/go-pluralize"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func (mongoCrawler *mongoCrawler) getMongodbDatabase(dbName string) database.Database {
	return database.Database{
		Name:    dbName,
		Host:    mongoCrawler.Host,
		Schemas: []string{},
	}
}

func (mongoCrawler *mongoCrawler) getMongodbDatabaseCollection(dbHandle *mongo.Database, collectionName string) (database.Table, error) {
	collectionHandle := dbHandle.Collection(collectionName)

	// get the collection indexes
	collectionIndexesNames, errListCollectionIndexesNames := mongoCrawler.listCollectionIndexesNames(collectionHandle)
	if errListCollectionIndexesNames != nil {
		// log here and do nothing else
		log.Error().Msgf("Could not retrieve index names for the collection %s. Error was: %s", collectionName, errListCollectionIndexesNames.Error())
	}

	// get collection columns
	collectionColumns, errCollectionColumns := mongoCrawler.getCollectionColumns(collectionHandle)
	if errCollectionColumns != nil {
		log.Error().Msgf("Could not retrieve columns for the collection %s. Error was: %s", collectionName, errCollectionColumns.Error())
	}

	// sort by column names
	sort.Slice(collectionColumns, func(i, j int) bool {
		return collectionColumns[i].Name < collectionColumns[j].Name
	})

	// get constraints
	collectionConstraints, errCollectionConstraints := mongoCrawler.getCollectionConstraints(collectionHandle)
	if errCollectionConstraints != nil {
		log.Error().Msgf("Could not retrieve constraints for the collection %s. Error was: %s", collectionName, errCollectionConstraints.Error())
	}

	return database.Table{
		Name:        fmt.Sprintf("%s.%s", dbHandle.Name(), collectionName),
		Columns:     collectionColumns,
		Indexes:     collectionIndexesNames,
		Constraints: collectionConstraints,
		Schema:      dbHandle.Name(),
	}, nil
}

func (mongoCrawler) listCollectionIndexesNames(collectionHandle *mongo.Collection) ([]string, error) {
	foundIndexes := []string{}
	indexesCursor, err := collectionHandle.Indexes().List(context.Background())
	if err != nil {
		return foundIndexes, err
	}

	var result []bson.M
	if err := indexesCursor.All(context.TODO(), &result); err != nil {
		log.Error().Msgf("Could not load indexes in the result to get the index names for the collection %s. Error was: %s", collectionHandle.Name(), err.Error())
	}

	for _, value := range result {
		for k, v := range value {
			if k == "name" {
				foundIndexes = append(foundIndexes, fmt.Sprintf("%s.%v", collectionHandle.Name(), v))
			}
		}
	}

	return foundIndexes, nil
}

func (mongoCrawler) listCollectionIndexes(collectionHandle *mongo.Collection) ([]database.Index, error) {
	dbName := collectionHandle.Database().Name()
	foundIndexes := []database.Index{}
	indexesCursor, err := collectionHandle.Indexes().List(context.Background())
	if err != nil {
		return foundIndexes, err
	}

	var result []bson.M
	if err = indexesCursor.All(context.TODO(), &result); err != nil {
		log.Error().Msgf("Could not load indexes in the result to get the index details for the collection %s. Error was: %s", collectionHandle.Name(), err.Error())
	}

	for _, value := range result {
		indexName := ""
		indexColumns := []database.Column{}

		for k, v := range value {
			switch k {
			case "name":
				indexName = fmt.Sprintf("%v", v)

			case "key":
				for key := range v.(bson.M) {
					indexColumns = append(indexColumns, database.Column{
						Name:     key,
						Type:     "",
						Position: -1, // not making use of it for the time being
					})
				}
			}
		}

		foundIndexes = append(foundIndexes, database.Index{
			Name:    fmt.Sprintf("%s.%s", collectionHandle.Name(), indexName),
			Columns: indexColumns,
			Table:   fmt.Sprintf("%s.%s", collectionHandle.Database().Name(), collectionHandle.Name()),
			Schema:  dbName,
		})
	}

	return foundIndexes, nil
}

func (mongoCrawler *mongoCrawler) getCollectionColumns(collection *mongo.Collection) ([]database.Column, error) {
	allFoundColumns := []database.Column{}
	pipeline := []bson.D{{{Key: "$sample", Value: bson.D{{Key: "size", Value: 64}}}}}
	cursor, err := collection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return allFoundColumns, err
	}

	for cursor.Next(context.Background()) {
		var result bson.D
		if err := cursor.Decode(&result); err != nil {
			return allFoundColumns, err
		}

		for key, value := range result.Map() {
			var valueType string

			switch value.(type) {
			case string:
				valueType = "string"
			case int64:
				valueType = "int64"
			case primitive.D:
				valueType = "document"
			case primitive.DateTime:
				valueType = "datetime"
			case primitive.A:
				valueType = "array"
			case primitive.ObjectID:
				valueType = "objectId"
			default:
				valueType = fmt.Sprintf("%T", value)
			}

			// check if column was already inserted
			columnExists := false
			for _, col := range allFoundColumns {
				if col.Name == key {
					columnExists = true
					break
				}
			}

			if !columnExists {
				allFoundColumns = append(allFoundColumns, database.Column{
					Name:     key,
					Type:     valueType,
					Position: -1,
				})
			}
		}
	}

	return allFoundColumns, nil
}

func (mongoCrawler *mongoCrawler) getCollectionConstraints(collection *mongo.Collection) ([]database.Constraint, error) {
	allConstraints := []database.Constraint{}

	pipeline := []bson.D{{{Key: "$sample", Value: bson.D{{Key: "size", Value: 64}}}}}
	cursor, err := collection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return allConstraints, err
	}

	for cursor.Next(context.Background()) {
		var result bson.D
		if err := cursor.Decode(&result); err != nil {
			return allConstraints, err
		}

		for key, value := range result.Map() {
			var valueType string
			var src []database.Column
			var dst []database.Column

			switch value.(type) {
			case primitive.ObjectID:
				if key == "_id" {
					// create primary key constraint on the column _id
					valueType = mongodb.MONGODB_CONSTRAINT_PK
					src = []database.Column{
						{
							Name:     "_id",
							Type:     "objectId",
							Position: 1,
						},
					}
					dst = []database.Column{}
				} else {
					// try to infer remote table
					allCollectionNames, errAllCollections := collection.Database().ListCollectionNames(context.Background(), bson.D{})
					if errAllCollections != nil {
						// if we cannot retrieve the collection names then we move on
						continue
					}

					potentialCollectionName := getCollectionNameFromColumnID(key)

					// check if this collection exists
					foundReferencedCollection := false
					indexFoundReferencedCollection := -1
					for index, existingCollection := range allCollectionNames {
						if strings.ToLower(existingCollection) == potentialCollectionName {
							foundReferencedCollection = true
							indexFoundReferencedCollection = index
							break
						}
					}

					if !foundReferencedCollection {
						continue
					}

					valueType = mongodb.MONGODB_CONSTRAINT_FK
					src = []database.Column{
						{
							Name:     fmt.Sprintf("%s_fk", potentialCollectionName),
							Type:     "objectId",
							Position: -1,
						},
					}

					dst = []database.Column{
						{
							Name:     fmt.Sprintf("%s.%s._id", collection.Database().Name(), allCollectionNames[indexFoundReferencedCollection]),
							Type:     "objectId",
							Position: 1,
						},
					}
				}

			default:
				continue
			}

			// check if column was already inserted
			columnExists := false
			for _, col := range allConstraints {
				if col.Name == key {
					columnExists = true
					break
				}
			}

			if !columnExists {
				allConstraints = append(allConstraints, database.Constraint{
					Name:         key,
					Type:         valueType,
					Sources:      src,
					Destinations: dst,
				})
			}
		}
	}

	return allConstraints, nil
}

// getCollectionNameFromColumnID tries to infer the collection name from a ID column. It tries to remove any _ and 'id' and the resulting string is
// pluralized and returned. In the case that no _ or 'id' were found then we return the same string.
func getCollectionNameFromColumnID(columnID string) string {
	loweCaseColumnID := strings.Trim(strings.ToLower(columnID), " ")
	columnIDWithoutUnderscore := strings.ReplaceAll(loweCaseColumnID, "_", "")
	columnIDWithoutUnderscoreAndID := strings.ReplaceAll(columnIDWithoutUnderscore, "id", "")

	if columnIDWithoutUnderscoreAndID == strings.ToLower(columnID) {

		return columnID
	}

	return pluralize.NewClient().Plural(columnIDWithoutUnderscoreAndID)
}
