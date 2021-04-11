{% import "../macros/subTags.tpl" sub_tags %}

<div class="person">
    <a href="/person/{{ person.ID }}">
        <h3>{{ person.Name }} {{ person.Surname }}</h3>
    </a>
    <p>{{ person.Description }}</p>
    <div class="tags mt-3 mb-2" style="margin-left: -0.5rem">
        {{ sub_tags(tags, person.Tags) }}
    </div>
</div>