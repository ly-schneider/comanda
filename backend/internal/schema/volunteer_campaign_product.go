package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type VolunteerCampaignProduct struct {
	ent.Schema
}

func (VolunteerCampaignProduct) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "volunteer_campaign_product"},
		field.ID("campaign_id", "product_id"),
	}
}

func (VolunteerCampaignProduct) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (VolunteerCampaignProduct) Fields() []ent.Field {
	return []ent.Field{
		field.String("campaign_id").MaxLen(12),
		field.String("product_id").MaxLen(12),
		field.Int("quantity").
			Default(1),
		// 0 = always included. A positive value groups products into alternatives:
		// exactly one product of that group is handed out per redemption.
		field.Int("choice_group").
			Default(0),
		// Splits a choice group into named-by-position alternatives, so
		// "Pommes ODER ein Getränk" keeps the drinks together as one branch
		// instead of flattening into a single list.
		field.Int("option_group").
			Default(0),
	}
}

func (VolunteerCampaignProduct) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("campaign", VolunteerCampaign.Type).
			Field("campaign_id").
			Unique().
			Required(),
		edge.To("product", Product.Type).
			Field("product_id").
			Unique().
			Required(),
	}
}
