# Test Case Format Design

## Status: Draft - Needs More Thinking

---

## Requirements

| Requirement | Description |
|-------------|-------------|
| AI can write | AI generates test cases |
| AI can heal | AI fixes broken tests |
| Self-healing | Multiple selectors, fallback without AI |
| Fast execution | No AI in runtime loop |
| Complex logic | Conditionals, loops, data-driven |
| Intent context | AI understands purpose of each step |

---

## User Workflow

```
1. AI writes test cases      → Uses MCP to explore, generates code
2. AI heals test cases       → When tests break, AI fixes them
3. Run test cases (CI/CD)    → Fast, cheap, no AI involved
```

---

## Proposed Approach: Code + Selectors + Intent

### Why Code?

- YAML/JSON fail for complex logic (conditionals, loops, data-driven)
- AI generates code very well
- Engineers already know code
- Full expressiveness

### Why Multiple Selectors?

- Self-healing without AI (try fallbacks)
- Resilient to UI changes
- Only call AI when all selectors fail

### Why Intent?

- Context for AI when healing
- Explains what step is trying to do
- Helps AI find correct new selector

---

## Syntax Design

### Option 1: Object Style

```typescript
await device.tap({
  selectors: ['Login', 'Sign In', 'id:login_btn'],
  intent: 'tap login button'
})

await device.type({
  selectors: ['Email', 'id:email_input'],
  value: 'test@example.com',
  intent: 'enter email'
})
```

### Option 2: Fluent Style

```typescript
await device.tap('Login')
  .or('Sign In')
  .or('id:login_btn')
  .intent('tap login button')
```

### Option 3: Array + String

```typescript
await device.tap(
  ['Login', 'Sign In', 'id:login_btn'],
  'tap login button'
)
```

### Option 4: Tagged Template

```typescript
await tap`
  selectors: Login | Sign In | id:login_btn
  intent: tap login button
`
```

---

## Full Example

```typescript
import { test, device } from 'maestro-runner'

test('user can login', async () => {
  await device.launch('com.example.app')

  // Handle optional onboarding
  if (await device.isVisible(['Skip', 'Skip Tutorial'])) {
    await device.tap({
      selectors: ['Skip', 'Skip Tutorial'],
      intent: 'skip onboarding'
    })
  }

  await device.tap({
    selectors: ['Login', 'Sign In', 'id:login_btn'],
    intent: 'tap login button on welcome screen'
  })

  await device.type({
    selectors: ['Email', 'Email Address', 'hint:Enter email', 'id:email_input'],
    value: 'test@example.com',
    intent: 'enter email in login form'
  })

  await device.type({
    selectors: ['Password', 'hint:Enter password', 'id:password_input'],
    value: 'secret123',
    intent: 'enter password'
  })

  await device.tap({
    selectors: ['Submit', 'Log In', 'Sign In', 'id:submit_btn'],
    intent: 'submit login form'
  })

  await device.assertVisible({
    selectors: ['Welcome', 'Home', 'id:welcome_screen'],
    intent: 'verify login succeeded'
  })
})
```

---

## Self-Healing Flow

```
Test Execution:

Step: tap login button
  ├── Try selector 1: "Login"      → NOT FOUND
  ├── Try selector 2: "Sign In"    → FOUND ✓
  └── Execute tap

Test passes. No AI needed.
```

```
When All Selectors Fail:

Step: tap login button
  ├── Try all selectors            → ALL FAILED
  └── SELF-HEAL MODE:
      • Capture screenshot
      • Capture hierarchy
      • Send to AI with intent
      • AI suggests new selector
      • Update test file
      • Retry step
```

---

## Selector Types

```typescript
selectors: [
  'Login',                         // text match (contains)
  'id:login_btn',                  // resource ID
  'desc:Sign in button',           // content description
  'hint:Enter email',              // hint text
  'class:android.widget.Button',   // class name
  '//*[@text="Login"]',            // xpath (last resort)
]
```

---

## Complex Logic Examples

### Conditional

```typescript
if (await device.isVisible(['Skip', 'id:skip'])) {
  await device.tap({
    selectors: ['Skip'],
    intent: 'skip tutorial'
  })
}
```

### Loop

```typescript
for (let i = 0; i < 5; i++) {
  await device.swipe('left')
}
```

### Data-Driven

```typescript
const users = [
  { email: 'a@test.com', pass: '123' },
  { email: 'b@test.com', pass: '456' },
]

for (const user of users) {
  await device.type({
    selectors: ['Email'],
    value: user.email,
    intent: 'enter email'
  })
  await device.type({
    selectors: ['Password'],
    value: user.pass,
    intent: 'enter password'
  })
  await device.tap({ selectors: ['Login'], intent: 'login' })
  await device.tap({ selectors: ['Logout'], intent: 'logout' })
}
```

### Reusable Helper

```typescript
async function login(email: string, password: string) {
  await device.tap({
    selectors: ['Login', 'Sign In'],
    intent: 'open login'
  })
  await device.type({
    selectors: ['Email'],
    value: email,
    intent: 'enter email'
  })
  await device.type({
    selectors: ['Password'],
    value: password,
    intent: 'enter password'
  })
  await device.tap({
    selectors: ['Submit'],
    intent: 'submit login'
  })
}

// Use in tests
test('checkout flow', async () => {
  await login('test@example.com', 'secret')
  // ... rest of test
})
```

---

## Open Questions

1. **Which syntax option?** Object, fluent, array, or template?

2. **Language?** TypeScript, JavaScript, or language-agnostic?

3. **How to store selectors learned by AI?** Inline update or separate file?

4. **Selector priority?** First match or best match?

5. **Self-heal mode trigger?** Auto-heal or prompt user?

6. **How to handle selector explosion?** Max selectors per step?

7. **Integration with existing frameworks?** Jest, Mocha, etc.?

8. **Cross-platform?** Same test for Android and iOS?

---

## Comparison with Existing Tools

| Aspect | Appium | Maestro | Proposed |
|--------|--------|---------|----------|
| Format | Code | YAML | Code |
| Selectors | Single | Single | Multiple (fallback) |
| Self-heal | No | No | Yes |
| Intent | No | No | Yes |
| Complex logic | Yes | Limited | Yes |
| AI integration | No | No | Built-in |

---

## Next Steps

- [ ] Decide on syntax style
- [ ] Design SDK API
- [ ] Prototype self-healing logic
- [ ] Test with real apps
- [ ] AI integration for healing
