# GraphQL query limits

The `/graphql` endpoint rejects queries deeper than 10 selection levels or with more than 1,000 selected fields. The structural cost check runs before resolver work, ignores braces in strings and comments, and returns HTTP 400 with a specific depth or complexity error. These limits bound deeply nested and unusually wide requests without changing normal query responses.
